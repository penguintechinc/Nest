package security

import "testing"

// TestCheckQuery_Branches covers the empty / pattern-match / heuristic / clean
// branches of the raw-bytes CheckQuery path.
func TestCheckQuery_Branches(t *testing.T) {
	c := NewChecker(nil)

	if blocked, _ := c.CheckQuery(""); blocked {
		t.Error("empty query should not be blocked")
	}
	if blocked, _ := c.CheckQuery("   "); blocked {
		t.Error("whitespace-only query should not be blocked")
	}

	// Clean query — should pass.
	if blocked, _ := c.CheckQuery("select id from users where id = 1"); blocked {
		t.Error("clean parameterised-style query should not be blocked")
	}

	// Classic injection pattern (OR 1=1 / UNION SELECT) — should trip a pattern.
	blocked, reason := c.CheckQuery("select * from users where name = '' or 1=1 --")
	if !blocked || reason == "" {
		t.Errorf("injection query should be blocked, got blocked=%v reason=%q", blocked, reason)
	}

	// Excessive-keyword heuristic.
	blocked2, _ := c.CheckQuery("select union insert update delete drop from where and or")
	if !blocked2 {
		t.Error("excessive-keyword query should be blocked by heuristic")
	}
}

// TestCheckParsedQuery_Branches covers the parsed-text path.
func TestCheckParsedQuery_Branches(t *testing.T) {
	c := NewChecker(nil)

	if blocked, _ := c.CheckParsedQuery(""); blocked {
		t.Error("empty parsed query should not be blocked")
	}
	if blocked, _ := c.CheckParsedQuery("select 1"); blocked {
		t.Error("trivial clean query should not be blocked")
	}
	if blocked, _ := c.CheckParsedQuery("'; drop table users; --"); !blocked {
		t.Error("stacked injection should be blocked")
	}
}
