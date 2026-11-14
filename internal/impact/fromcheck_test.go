package impact

import "testing"

func TestFromCheck(t *testing.T) {
	in := []byte(`{"violations":[{"endpoint":"GET /a/{id} 200","path":"$.x","type":"POSSIBLE_RENAME","severity":"BREAKING","rename_from":"$.x","rename_to":"$.y"},{"endpoint":"GET /a/{id} 200","path":"$.new","type":"ADDED_KEY","severity":"SAFE"}]}`)
	got, err := FromCheck(in)
	if err != nil || len(got) != 1 || got[0].Path != "$.x" {
		t.Fatalf("got %v, err %v", got, err)
	}
}
