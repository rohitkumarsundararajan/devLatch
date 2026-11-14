import { Project, Node, SyntaxKind } from "ts-morph";
import path from "node:path";

const VERBS = new Set(["get", "post", "put", "patch", "delete", "head", "request"]);
const CALLBACK_METHODS = new Set(["map", "forEach", "filter", "find", "flatMap", "some", "every", "then"]);
const RANK = { "type-linked": 0, "flow-linked": 1, "name-only": 2 };

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

function parseEndpoint(ep) {
  const m = ep.trim().match(/^([A-Za-z]+)\s+(\S+)/);
  const method = m && /^[A-Z]+$/.test(m[1]) ? m[1] : null;
  const p = (method ? m[2] : ep.trim()).split("?")[0];
  const re = new RegExp(p.split(/\{[^}]+\}/).map(escapeRe).join("[^/]+") + "/?$");
  return { method, re };
}

function urlText(n) {
  if (!n) return null;
  if (Node.isStringLiteral(n) || Node.isNoSubstitutionTemplateLiteral(n)) return n.getLiteralText();
  if (Node.isTemplateExpression(n)) {
    return n.getHead().getLiteralText() + n.getTemplateSpans().map((s) => "X" + s.getLiteral().getLiteralText()).join("");
  }
  if (Node.isBinaryExpression(n) && n.getOperatorToken().getKind() === SyntaxKind.PlusToken) {
    return (urlText(n.getLeft()) ?? "X") + (urlText(n.getRight()) ?? "X");
  }
  return null;
}

const isFn = (n) =>
  Node.isFunctionDeclaration(n) || Node.isArrowFunction(n) || Node.isFunctionExpression(n) || Node.isMethodDeclaration(n);

function bindingNames(nameNode) {
  if (Node.isIdentifier(nameNode)) return [nameNode.getText()];
  return nameNode
    .getDescendantsOfKind(SyntaxKind.BindingElement)
    .map((b) => b.getNameNode())
    .filter(Node.isIdentifier)
    .map((i) => i.getText());
}

function wrapperCalls(fn) {
  let nameNode = null;
  if (Node.isFunctionDeclaration(fn)) nameNode = fn.getNameNode();
  else {
    const p = fn.getParent();
    if (p && Node.isVariableDeclaration(p)) nameNode = p.getNameNode();
  }
  if (!nameNode || !Node.isIdentifier(nameNode)) return [];
  return nameNode
    .findReferencesAsNodes()
    .filter((r) => {
      const p = r.getParent();
      return p && Node.isCallExpression(p) && p.getExpression() === r;
    })
    .map((r) => r.getParent());
}

function findEndpointCalls(files, ep) {
  const { method, re } = parseEndpoint(ep);
  const out = [];
  for (const sf of files) {
    for (const call of sf.getDescendantsOfKind(SyntaxKind.CallExpression)) {
      const t = urlText(call.getArguments()[0]);
      if (t == null || !re.test(t.split("?")[0])) continue;
      const callee = call.getExpression();
      if (Node.isPropertyAccessExpression(callee)) {
        const nm = callee.getName();
        if (VERBS.has(nm) && nm !== "request" && method && nm.toUpperCase() !== method) continue;
      }
      out.push(call);
    }
  }
  return out;
}

function taintFor(call) {
  const scope = call.getFirstAncestor(isFn) ?? call.getSourceFile();
  const names = new Set();
  const vars = [];
  const inCall = (n) => n.getSourceFile() === call.getSourceFile() && n.getPos() <= call.getPos() && call.getEnd() <= n.getEnd();
  const hasTainted = (n) =>
    Node.isIdentifier(n) ? names.has(n.getText()) : n.getDescendantsOfKind(SyntaxKind.Identifier).some((i) => names.has(i.getText()));
  for (let pass = 0; pass < 3; pass++) {
    scope.forEachDescendant((n) => {
      if (Node.isVariableDeclaration(n)) {
        const init = n.getInitializer();
        if (init && (inCall(init) || hasTainted(init))) {
          bindingNames(n.getNameNode()).forEach((x) => names.add(x));
          if (!vars.includes(n)) vars.push(n);
        }
      } else if (Node.isCallExpression(n)) {
        const ex = n.getExpression();
        if (Node.isPropertyAccessExpression(ex) && CALLBACK_METHODS.has(ex.getName())) {
          const recv = ex.getExpression();
          if (inCall(recv) || hasTainted(recv)) {
            const cb = n.getArguments()[0];
            if (cb && (Node.isArrowFunction(cb) || Node.isFunctionExpression(cb))) {
              const p = cb.getParameters()[0];
              if (p) bindingNames(p.getNameNode()).forEach((x) => names.add(x));
            }
          }
        }
      }
    });
  }
  return { scope, names, vars };
}

function collectLinked(types, isProject, out) {
  const seen = new Set();
  let budget = 3000;
  const visit = (t, depth) => {
    if (!t || depth > 6 || budget-- <= 0) return;
    const id = t.compilerType.id;
    if (seen.has(id)) return;
    seen.add(id);
    if (t.isUnion()) return t.getUnionTypes().forEach((u) => visit(u, depth));
    if (t.isArray()) return visit(t.getArrayElementType(), depth + 1);
    const sym = t.getSymbol();
    if (!sym) return;
    if (sym.getName() === "Promise") return t.getTypeArguments().forEach((a) => visit(a, depth));
    const external = sym.getDeclarations().every((d) => !isProject(d.getSourceFile()));
    if (external) {
      t.getTypeArguments().forEach((a) => visit(a, depth + 1));
      const data = t.getProperty("data");
      if (data) visit(data.getTypeAtLocation(data.getDeclarations()[0]), depth + 1);
      return;
    }
    for (const p of t.getProperties()) {
      const decls = p.getDeclarations();
      decls.forEach((d) => isProject(d.getSourceFile()) && out.add(d));
      if (decls[0]) visit(p.getTypeAtLocation(decls[0]), depth + 1);
    }
  };
  types.forEach((t) => visit(t, 0));
}

function propDecls(node, key) {
  try {
    if (Node.isPropertyAccessExpression(node)) return node.getNameNode().getSymbol()?.getDeclarations() ?? [];
    if (Node.isElementAccessExpression(node)) return node.getExpression().getType().getProperty(key)?.getDeclarations() ?? [];
    if (Node.isBindingElement(node)) return node.getParent().getType().getProperty(key)?.getDeclarations() ?? [];
  } catch {}
  return [];
}

function scanOne(files, isProject, drift, cwd) {
  const segs = drift.path.replace(/^\$\.?/, "").split(".").map((s) => s.replace(/\[\]$/, "")).filter(Boolean);
  const key = segs[segs.length - 1];
  if (!key) return { endpointCalls: 0, hits: [] };

  // 1. endpoint calls, plus wrapper functions around them (2 hops)
  const sources = findEndpointCalls(files, drift.endpoint).map((call) => ({ call, wrapper: false }));
  const endpointCalls = sources.length;
  for (let i = 0, depth = 0; depth < 2; depth++) {
    const end = sources.length;
    for (; i < end; i++) {
      const fn = sources[i].call.getFirstAncestor(isFn);
      if (fn) wrapperCalls(fn).forEach((c) => sources.push({ call: c, wrapper: true }));
    }
  }

  // 2. taint (flow) + linked declarations (types)
  const taints = sources.map((s) => ({ ...taintFor(s.call) }));
  const types = [];
  sources.forEach((s, i) => {
    s.call.getTypeArguments().forEach((t) => types.push(t.getType()));
    if (s.wrapper) types.push(s.call.getType());
    else {
      const fn = s.call.getFirstAncestor(isFn);
      if (fn) types.push(fn.getReturnType());
    }
    taints[i].vars.forEach((v) => types.push(v.getType()));
  });
  const linked = new Set();
  collectLinked(types, isProject, linked);

  const isTainted = (node) => {
    let expr = null;
    if (Node.isPropertyAccessExpression(node) || Node.isElementAccessExpression(node)) expr = node.getExpression();
    else if (Node.isBindingElement(node)) {
      const d = node.getParent()?.getParent();
      if (d && Node.isVariableDeclaration(d)) expr = d.getInitializer();
    }
    if (!expr) return false;
    const ids = Node.isIdentifier(expr) ? [expr] : expr.getDescendantsOfKind(SyntaxKind.Identifier);
    return taints.some(
      (t) => t.scope.getSourceFile() === node.getSourceFile() && t.scope.getPos() <= node.getPos() && node.getEnd() <= t.scope.getEnd() && ids.some((i) => t.names.has(i.getText()))
    );
  };

  // 3. find every place the key appears and classify it
  const hits = new Map();
  for (const sf of files) {
    sf.forEachDescendant((node) => {
      let kind = null;
      if (Node.isPropertyAccessExpression(node) && node.getName() === key) kind = "read";
      else if (Node.isElementAccessExpression(node)) {
        const a = node.getArgumentExpression();
        if (a && Node.isStringLiteral(a) && a.getLiteralText() === key) kind = "read";
      } else if (Node.isBindingElement(node) && Node.isObjectBindingPattern(node.getParent())) {
        if ((node.getPropertyNameNode() ?? node.getNameNode()).getText() === key) kind = "destructure";
      } else if (Node.isPropertySignature(node) && node.getName() === key) kind = "declaration";
      else if (Node.isPropertyAssignment(node) && node.getName() === key) {
        const c = node.getFirstAncestorByKind(SyntaxKind.CallExpression);
        if (c && c.getExpression().getText().startsWith("z.")) kind = "declaration";
      }
      if (!kind) return;
      let confidence = "name-only";
      if (kind === "declaration") {
        if (linked.has(node)) confidence = "type-linked";
      } else if (propDecls(node, key).some((d) => linked.has(d))) confidence = "type-linked";
      else if (isTainted(node)) confidence = "flow-linked";
      const line = node.getStartLineNumber();
      const file = path.relative(cwd, sf.getFilePath()).split(path.sep).join("/");
      const id = `${file}:${line}:${kind}`;
      const prev = hits.get(id);
      if (prev && RANK[prev.confidence] <= RANK[confidence]) return;
      const snippet = sf.getFullText().split("\n")[line - 1].trim().slice(0, 120);
      hits.set(id, { file, line, kind, confidence, snippet });
    });
  }
  const sorted = [...hits.values()].sort(
    (a, b) => RANK[a.confidence] - RANK[b.confidence] || a.file.localeCompare(b.file) || a.line - b.line
  );
  return { endpointCalls, hits: sorted };
}

export function scan({ tsconfig, drifts, cwd = process.cwd() }) {
  const project = new Project({ tsConfigFilePath: path.resolve(tsconfig) });
  const files = project.getSourceFiles().filter((f) => !f.getFilePath().includes("/node_modules/") && !f.isDeclarationFile());
  const set = new Set(files.map((f) => f.getFilePath()));
  const isProject = (sf) => set.has(sf.getFilePath());
  return drifts.map((d) => ({ ...d, ...scanOne(files, isProject, d, cwd) }));
}