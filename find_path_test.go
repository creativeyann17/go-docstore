package docstore

import "testing"

type historyDoc struct {
	Login   string `json:"login"`
	Address struct {
		City string `json:"city"`
	} `json:"address"`
	History []struct {
		At string `json:"at"`
	} `json:"loginHistory"`
	Tags []string `json:"tags"`
}

func TestFindPathPlainField(t *testing.T) {
	c := coll(t, open(t))
	c.Put("a", doc{Login: "yann"})
	c.Put("b", doc{Login: "bob"})

	hits, err := c.FindPath("login", "=", "bob", 0)
	if err != nil || len(hits) != 1 || hits[0].ID != "b" {
		t.Fatalf("plain field: %v %v", err, hits)
	}
}

func TestFindPathNestedObject(t *testing.T) {
	c := coll(t, open(t))
	var d historyDoc
	d.Login = "yann"
	d.Address.City = "Montreal"
	c.Put("a", d)

	hits, err := c.FindPath("address.city", "=", "Montreal", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("nested object: %v %v", err, hits)
	}
}

// TestFindPathArrayOfObjectsMatchesAnyEntryNotJustFirst is the exact
// regression this feature exists for: a document whose match is on
// the SECOND array entry, not the first, must still be found.
func TestFindPathArrayOfObjectsMatchesAnyEntryNotJustFirst(t *testing.T) {
	c := coll(t, open(t))
	var d historyDoc
	d.Login = "yann"
	d.History = []struct {
		At string `json:"at"`
	}{{At: "2026-01-01"}, {At: "2026-06-15"}}
	c.Put("a", d)

	hits, err := c.FindPath("loginHistory.at", "=", "2026-06-15", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("match on entry[1] not found: %v %v", err, hits)
	}
	// The old behavior (literal $.loginHistory[0].at) would have found
	// entry 0 only — prove entry 0's value does NOT equal what we
	// searched, so this test can't accidentally pass for the wrong reason.
	if d.History[0].At == "2026-06-15" {
		t.Fatal("test setup invalid: entry 0 must not equal the search value")
	}

	miss, err := c.FindPath("loginHistory.at", "=", "1999-01-01", 0)
	if err != nil || len(miss) != 0 {
		t.Fatalf("false positive: %v %v", err, miss)
	}
}

func TestFindPathArrayOfScalarsMembership(t *testing.T) {
	c := coll(t, open(t))
	var d historyDoc
	d.Login = "yann"
	d.Tags = []string{"admin", "beta"}
	c.Put("a", d)

	hits, err := c.FindPath("tags", "=", "beta", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("scalar array membership: %v %v", err, hits)
	}
	miss, err := c.FindPath("tags", "=", "nope", 0)
	if err != nil || len(miss) != 0 {
		t.Fatalf("false positive: %v %v", err, miss)
	}
}

func TestFindPathMissingFieldNoMatchNoError(t *testing.T) {
	c := coll(t, open(t))
	c.Put("a", doc{Login: "yann"})

	hits, err := c.FindPath("address.city", "=", "Montreal", 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("missing field: %v %v", err, hits)
	}
}

func TestFindPathOperatorsAndDollarPrefixTolerated(t *testing.T) {
	c := coll(t, open(t))
	c.Put("a", doc{N: 10})
	c.Put("b", doc{N: 20})

	gt, err := c.FindPath("n", ">", 15, 0)
	if err != nil || len(gt) != 1 || gt[0].ID != "b" {
		t.Fatalf("> op: %v %v", err, gt)
	}
	// Leading "$." is tolerated (muscle memory from the old Find syntax).
	dollar, err := c.FindPath("$.n", ">", 15, 0)
	if err != nil || len(dollar) != 1 {
		t.Fatalf("$-prefixed path: %v %v", err, dollar)
	}
	if _, err := c.FindPath("n", "BOGUS", 1, 0); err == nil {
		t.Fatal("invalid op accepted")
	}
	if _, err := c.FindPath("", "=", 1, 0); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestFindPathRespectsSoftDelete(t *testing.T) {
	c := coll(t, open(t), WithSoftDelete())
	c.Put("a", doc{Login: "yann"})
	c.Delete("a")

	hits, err := c.FindPath("login", "=", "yann", 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("soft-deleted doc matched: %v %v", err, hits)
	}
}
