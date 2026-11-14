package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
)

// Mutation names. Each one maps to a drift case from the DevLatch blueprint
// (section 15, verification scenarios).
const (
	MutRenameUserName   = "rename_user_name"  // user_name -> username      (field rename)
	MutBalanceToString  = "balance_to_string" // balance NUMBER -> STRING   (type change)
	MutTagsToObjects    = "tags_to_objects"   // tags []string -> []object  (array element change)
	MutRemoveEmail      = "remove_email"      // required key disappears    (removed key)
	MutNullableNickname = "nullable_nickname" // nickname becomes null      (new nullability)
	MutAddCurrency      = "add_currency"      // orders gain "currency"     (added key, safe)
	MutNewMapDate       = "new_map_date"      // by_day gains a date        (MAP, must NOT alarm)
)

var allMutations = []string{
	MutAddCurrency,
	MutBalanceToString,
	MutNewMapDate,
	MutNullableNickname,
	MutRemoveEmail,
	MutRenameUserName,
	MutTagsToObjects,
}

const (
	userCount = 20
	pageSize  = 10
)

func validMutation(name string) bool {
	for _, m := range allMutations {
		if m == name {
			return true
		}
	}
	return false
}

type server struct {
	mu     sync.RWMutex
	active map[string]bool
}

func newServer() *server {
	return &server{active: map[string]bool{}}
}

func (s *server) has(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[name]
}

func (s *server) set(name string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.active[name] = true
	} else {
		delete(s.active, name)
	}
}

func (s *server) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = map[string]bool{}
}

func (s *server) activeList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.active))
	for name := range s.active {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)

	// API under test.
	mux.HandleFunc("GET /api/v1/users", s.listUsers)
	mux.HandleFunc("GET /api/v1/users/{id}", s.getUser)
	mux.HandleFunc("GET /api/v1/orders", s.listOrders)
	mux.HandleFunc("GET /api/v1/stats/daily", s.dailyStats)

	// Demo controls (never proxied in real use; DevLatch should ignore /__demo).
	mux.HandleFunc("GET /__demo/mutations", s.listMutations)
	mux.HandleFunc("POST /__demo/mutations/{name}", s.enableMutation)
	mux.HandleFunc("DELETE /__demo/mutations/{name}", s.disableMutation)
	mux.HandleFunc("POST /__demo/reset", s.resetMutations)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// userObject builds user n (1-based) with the active mutations applied.
//
// Baseline shape: id STRING, email STRING, role_id INTEGER, nickname STRING,
// permissions []STRING, user_name STRING, phone STRING (only for even ids,
// so it is an optional field).
func (s *server) userObject(n int) map[string]any {
	name := "user" + strconv.Itoa(n)
	u := map[string]any{
		"id":          strconv.Itoa(n),
		"email":       name + "@example.com",
		"role_id":     n%3 + 1,
		"nickname":    "nick" + strconv.Itoa(n),
		"permissions": []string{"read", "write"},
	}
	if s.has(MutRenameUserName) {
		u["username"] = name
	} else {
		u["user_name"] = name
	}
	if n%2 == 0 {
		u["phone"] = fmt.Sprintf("+91-90000-%05d", n)
	}
	if s.has(MutRemoveEmail) {
		delete(u, "email")
	}
	if s.has(MutNullableNickname) {
		u["nickname"] = nil
	}
	return u
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || n < 1 || n > userCount {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found", "code": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, s.userObject(n))
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	users := []map[string]any{} // non-nil so an empty page encodes as [] not null
	for n := (page-1)*pageSize + 1; n <= page*pageSize && n <= userCount; n++ {
		users = append(users, s.userObject(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": page, "total": userCount, "users": users})
}

func (s *server) listOrders(w http.ResponseWriter, _ *http.Request) {
	seed := []struct {
		id      string
		balance float64
		items   int
		tags    []string
	}{
		{"o-1", 104.5, 2, []string{"gift", "express"}},
		{"o-2", 99.99, 1, []string{"standard"}},
		{"o-3", 250.25, 4, []string{}}, // empty array on purpose (element type unknown)
	}
	orders := make([]map[string]any, 0, len(seed))
	for _, o := range seed {
		var balance any = o.balance
		if s.has(MutBalanceToString) {
			balance = fmt.Sprintf("%.2f", o.balance)
		}
		var tags any = o.tags
		if s.has(MutTagsToObjects) {
			objs := make([]map[string]any, 0, len(o.tags))
			for _, t := range o.tags {
				objs = append(objs, map[string]any{"label": t})
			}
			tags = objs
		}
		order := map[string]any{"id": o.id, "balance": balance, "items": o.items, "tags": tags}
		if s.has(MutAddCurrency) {
			order["currency"] = "INR"
		}
		orders = append(orders, order)
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": 1, "orders": orders})
}

func (s *server) dailyStats(w http.ResponseWriter, _ *http.Request) {
	// Fixed dates keep responses deterministic. The keys are dynamic on purpose:
	// DevLatch must model this as a MAP, not a fixed property list.
	days := []string{"2026-11-01", "2026-11-02", "2026-11-03", "2026-11-04", "2026-11-05", "2026-11-06", "2026-11-07"}
	if s.has(MutNewMapDate) {
		days = append(days, "2026-11-08")
	}
	byDay := make(map[string]any, len(days))
	for i, d := range days {
		byDay[d] = map[string]any{"orders": 10 + i, "revenue": float64(100+i*10) + 0.5}
	}
	writeJSON(w, http.StatusOK, map[string]any{"by_day": byDay})
}

func (s *server) controlState() map[string]any {
	return map[string]any{"active": s.activeList(), "available": allMutations}
}

func (s *server) listMutations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.controlState())
}

func (s *server) enableMutation(w http.ResponseWriter, r *http.Request) {
	s.toggle(w, r, true)
}

func (s *server) disableMutation(w http.ResponseWriter, r *http.Request) {
	s.toggle(w, r, false)
}

func (s *server) toggle(w http.ResponseWriter, r *http.Request, on bool) {
	name := r.PathValue("name")
	if !validMutation(name) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown mutation", "available": allMutations})
		return
	}
	s.set(name, on)
	writeJSON(w, http.StatusOK, s.controlState())
}

func (s *server) resetMutations(w http.ResponseWriter, _ *http.Request) {
	s.reset()
	writeJSON(w, http.StatusOK, s.controlState())
}
