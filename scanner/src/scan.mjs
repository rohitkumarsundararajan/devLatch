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

function urlText(n, depth = 0) {
  if (!n || depth > 4) return null;
  if (Node.isStringLiteral(n) || Node.isNoSubstitutionTemplateLiteral(n)) return n.getLiteralText();
  if (Node.isTemplateExpression(n)) {
    return n.getHead().getLiteralText() + n.getTemplateSpans().map((s) => "X" + s.getLiteral().getLiteralText()).join("");
  }
  if (Node.isBinaryExpression(n) && n.getOperatorToken().getKind() === SyntaxKind.PlusToken) {
    return (urlText(n.getLeft(), depth + 1) ?? "X") + (urlText(n.getRight(), depth + 1) ?? "X");
  }
  return resolveUrl(n, depth);
}

// Follow a URL through const variables, object members and tiny helper functions.
function resolveUrl(n, depth) {
  try {
    if (Node.isParenthesizedExpression(n) || Node.isAsExpression(n) || Node.isNonNullExpression(n)) return urlText(n.getExpression(), depth + 1);
    if (Node.isIdentifier(n) || Node.isPropertyAccessExpression(n)) {
      for (const d of n.getSymbol()?.getDeclarations() ?? []) {
        if ((Node.isVariableDeclaration(d) || Node.isPropertyAssignment(d)) && d.getInitializer()) {
          const t = urlText(d.getInitializer(), depth + 1);
          if (t != null) return t;
        }
      }
      return null;
    }
    if (Node.isCallExpression(n)) {
      const fn = resolveFn(n.getExpression());
      const body = fn && fn.getBody ? fn.getBody() : null;
      if (!body) return null;
      if (!Node.isBlock(body)) return urlText(body, depth + 1);
      const ret = body.getDescendantsOfKind(SyntaxKind.ReturnStatement)[0];
      return ret ? urlText(ret.getExpression(), depth + 1) : null;
    }
  } catch {}
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
  if (Node.isFunctionDeclaration(fn) || Node.isMethodDeclaration(fn)) nameNode = fn.getNameNode();
  else {
    const p = fn.getParent();
    if (p && (Node.isVariableDeclaration(p) || Node.isPropertyAssignment(p))) nameNode = p.getNameNode();
  }
  if (!nameNode || !Node.isIdentifier(nameNode)) return [];
  const calls = [];
  for (const r of nameNode.findReferencesAsNodes()) {
    const p = r.getParent();
    if (p && Node.isCallExpression(p) && p.getExpression() === r) calls.push(p);
    else if (p && Node.isPropertyAccessExpression(p) && p.getNameNode() === r) {
      const g = p.getParent();
      if (g && Node.isCallExpression(g) && g.getExpression() === p) calls.push(g);
    }
  }
  return calls;
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
  const inCall = (n) => n.getSourceFile() === call.getSourceFile() && n.getPos() <= call.getPos() && call.getEnd() <= n.getEnd();
  return taintFlow(scope, new Set(), inCall);
}

function taintFlow(scope, names, inCall) {
  const vars = [];
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
      } else if (Node.isForOfStatement(n)) {
        const e = n.getExpression();
        if (inCall(e) || hasTainted(e)) {
          const init = n.getInitializer();
          if (Node.isVariableDeclarationList(init)) init.getDeclarations().forEach((d) => bindingNames(d.getNameNode()).forEach((x) => names.add(x)));
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

function resolveFn(expr) {
  try {
    for (const d of expr.getSymbol()?.getDeclarations() ?? []) {
      if (isFn(d)) return d;
      if (Node.isVariableDeclaration(d) || Node.isPropertyAssignment(d)) {
        const i = d.getInitializer();
        if (i && isFn(i)) return i;
      }
    }
  } catch {}
  return null;
}

// When a tainted value is passed to a project function, taint that function's parameter (up to maxHops deep).
function propagateArgs(taints, isProject, maxHops = 3) {
  const seen = new Set();
  for (let i = 0; i < taints.length && taints.length < 300; i++) {
    const t = taints[i];
    const hop = t.hop ?? 0;
    if (hop >= maxHops) continue;
    const hasTainted = (n) =>
      Node.isIdentifier(n) ? t.names.has(n.getText()) : n.getDescendantsOfKind(SyntaxKind.Identifier).some((x) => t.names.has(x.getText()));
    for (const call of t.scope.getDescendantsOfKind(SyntaxKind.CallExpression)) {
      const fn = resolveFn(call.getExpression());
      if (!fn || !isProject(fn.getSourceFile())) continue;
      call.getArguments().forEach((arg, idx) => {
        if (!hasTainted(arg)) return;
        const p = fn.getParameters()[idx];
        if (!p) return;
        const key = `${fn.getSourceFile().getFilePath()}:${fn.getPos()}:${idx}`;
        if (seen.has(key)) return;
        seen.add(key);
        taints.push({ ...taintFlow(fn, new Set(bindingNames(p.getNameNode())), () => false), hop: hop + 1 });
      });
    }
  }
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

// Exact text edit that renames the field at this node (null when the node has no editable name).
function editFor(node, kind, to) {
  const inner = (n) => (Node.isStringLiteral(n) ? { start: n.getStart() + 1, end: n.getEnd() - 1, text: to } : { start: n.getStart(), end: n.getEnd(), text: to });
  let e = null;
  if (Node.isPropertyAccessExpression(node)) e = inner(node.getNameNode());
  else if (Node.isElementAccessExpression(node)) {
    const a = node.getArgumentExpression();
    if (a && Node.isStringLiteral(a)) e = inner(a);
  } else if (Node.isBindingElement(node)) {
    const pn = node.getPropertyNameNode();
    if (pn) e = inner(pn);
    else e = { start: node.getNameNode().getStart(), end: node.getNameNode().getStart(), text: to + ": " };
  } else if (Node.isPropertySignature(node) || Node.isPropertyAssignment(node)) e = inner(node.getNameNode());
  if (!e) return null;
  return { ...e, old: node.getSourceFile().getFullText().slice(e.start, e.end) };
}

// Pick<User, "field"> / Omit<User, "field">: string-literal keys that name the field on a linked type.
function typeKeyEdits(files, key, to, linked, edits, cwd) {
  for (const sf of files) {
    for (const lit of sf.getDescendantsOfKind(SyntaxKind.StringLiteral)) {
      if (lit.getLiteralText() !== key || !lit.getParent() || !Node.isLiteralTypeNode(lit.getParent())) continue;
      const ref = lit.getFirstAncestorByKind(SyntaxKind.TypeReference);
      if (!ref || !["Pick", "Omit"].includes(ref.getTypeName().getText())) continue;
      const base = ref.getTypeArguments()[0];
      let ok = false;
      try {
        ok = (base?.getType().getProperty(key)?.getDeclarations() ?? []).some((d) => linked.has(d));
      } catch {}
      if (!ok) continue;
      const start = lit.getStart() + 1;
      const end = lit.getEnd() - 1;
      const file = path.relative(cwd, sf.getFilePath()).split(path.sep).join("/");
      edits.set(sf.getFilePath() + ":" + start, { abs: sf.getFilePath(), file, line: lit.getStartLineNumber(), confidence: "type-linked", start, end, text: to, old: key });
    }
  }
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
  propagateArgs(taints, isProject);

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
  const edits = new Map();
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
      if (drift.to) {
        const ed = editFor(node, kind, drift.to);
        if (ed) edits.set(sf.getFilePath() + ":" + ed.start, { abs: sf.getFilePath(), file, line, confidence, ...ed });
      }
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
  if (drift.to) typeKeyEdits(files, key, drift.to, linked, edits, cwd);
  return { endpointCalls, hits: sorted, edits: [...edits.values()] };
}

export function scan({ tsconfig, drifts, cwd = process.cwd() }) {
  const project = new Project({ tsConfigFilePath: path.resolve(tsconfig) });
  const files = project.getSourceFiles().filter((f) => !f.getFilePath().includes("/node_modules/") && !f.isDeclarationFile());
  const set = new Set(files.map((f) => f.getFilePath()));
  const isProject = (sf) => set.has(sf.getFilePath());
  return drifts.map((d) => ({ ...d, ...scanOne(files, isProject, d, cwd) }));
}