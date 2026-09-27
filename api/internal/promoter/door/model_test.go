package door

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const at = "2026-10-03T23:15:00Z"

func TestOpValidation(t *testing.T) {
	guest := &Subject{Kind: KindGuest, ID: uuid.NewString()}
	cases := []struct {
		name string
		op   Op
		want string // problem prefix; "" = valid
	}{
		{"checkin", Op{Nonce: "n-000001", Type: OpCheckin, Subject: guest, Count: 1, Direction: DirIn, At: at}, ""},
		{"checkin out of a ticket", Op{Nonce: "n-000001", Type: OpCheckin, Subject: &Subject{Kind: KindTicket, ID: uuid.NewString()}, Count: 2, Direction: DirOut, At: at}, ""},
		{"undo", Op{Nonce: "n-000002", Type: OpUndo, Target: "n-000001", At: at}, ""},
		{"counter", Op{Nonce: "n-000003", Type: OpCounter, Kind: CounterWalkup, Delta: 3, At: at}, ""},
		{"short nonce", Op{Nonce: "n-1", Type: OpCounter, Kind: CounterIn, Delta: 1, At: at}, "nonce"},
		{"long nonce", Op{Nonce: strings.Repeat("x", 65), Type: OpCounter, Kind: CounterIn, Delta: 1, At: at}, "nonce"},
		{"nonce with a space", Op{Nonce: "n 000001", Type: OpCounter, Kind: CounterIn, Delta: 1, At: at}, "nonce"},
		{"missing at", Op{Nonce: "n-000001", Type: OpCounter, Kind: CounterIn, Delta: 1}, "at"},
		{"bad at", Op{Nonce: "n-000001", Type: OpCounter, Kind: CounterIn, Delta: 1, At: "yesterday"}, "at"},
		{"unknown type", Op{Nonce: "n-000001", Type: "delete", At: at}, "type"},
		{"no subject", Op{Nonce: "n-000001", Type: OpCheckin, Count: 1, Direction: DirIn, At: at}, "subject.kind"},
		{"bad subject kind", Op{Nonce: "n-000001", Type: OpCheckin, Subject: &Subject{Kind: "order", ID: uuid.NewString()}, Count: 1, Direction: DirIn, At: at}, "subject.kind"},
		{"bad subject id", Op{Nonce: "n-000001", Type: OpCheckin, Subject: &Subject{Kind: KindGuest, ID: "42"}, Count: 1, Direction: DirIn, At: at}, "subject.id"},
		{"zero count", Op{Nonce: "n-000001", Type: OpCheckin, Subject: guest, Direction: DirIn, At: at}, "count"},
		{"count over 50", Op{Nonce: "n-000001", Type: OpCheckin, Subject: guest, Count: 51, Direction: DirIn, At: at}, "count"},
		{"bad direction", Op{Nonce: "n-000001", Type: OpCheckin, Subject: guest, Count: 1, Direction: "up", At: at}, "direction"},
		{"checkin with counter fields", Op{Nonce: "n-000001", Type: OpCheckin, Subject: guest, Count: 1, Direction: DirIn, Delta: 1, At: at}, "checkin ops"},
		{"undo without target", Op{Nonce: "n-000002", Type: OpUndo, At: at}, "target"},
		{"undo of itself", Op{Nonce: "n-000002", Type: OpUndo, Target: "n-000002", At: at}, "target"},
		{"undo with a subject", Op{Nonce: "n-000002", Type: OpUndo, Target: "n-000001", Subject: guest, At: at}, "undo ops"},
		{"counter kind", Op{Nonce: "n-000003", Type: OpCounter, Kind: "vip", Delta: 1, At: at}, "kind"},
		{"counter delta", Op{Nonce: "n-000003", Type: OpCounter, Kind: CounterOut, Delta: 0, At: at}, "delta"},
		{"counter with a direction", Op{Nonce: "n-000003", Type: OpCounter, Kind: CounterOut, Delta: 1, Direction: DirIn, At: at}, "counter ops"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, problem := c.op.parse()
			if c.want == "" {
				if problem != "" {
					t.Fatalf("valid op rejected: %s", problem)
				}
				if p.at.IsZero() || (c.op.Type == OpCheckin && p.subjectID == uuid.Nil) {
					t.Fatalf("typed fields not set: %+v", p)
				}
				return
			}
			if !strings.HasPrefix(problem, c.want) {
				t.Fatalf("problem %q, want prefix %q", problem, c.want)
			}
		})
	}
}

func TestConflicts(t *testing.T) {
	cases := []struct {
		name      string
		allowance int
		count     int
		t         tally
		want      bool
	}{
		{"first entry", 1, 1, tally{}, false},
		{"same guest on two devices", 1, 1, tally{OtherIn: 1}, true},
		{"+2 split across devices", 3, 2, tally{OtherIn: 1}, false},
		{"+2 over-admitted across devices", 3, 2, tally{OtherIn: 2}, true},
		{"re-entry after an out elsewhere", 1, 1, tally{OtherIn: 1, Out: 1}, false},
		{"one device admits twice: its own decision", 1, 1, tally{OwnIn: 1}, false},
		{"own and other heads add up", 2, 1, tally{OtherIn: 1, OwnIn: 1}, true},
		{"ticket scanned on two devices", 1, 1, tally{OtherIn: 1, OwnIn: 0}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := conflicts(c.allowance, c.count, c.t); got != c.want {
				t.Fatalf("conflicts(%d, %d, %+v) = %v, want %v", c.allowance, c.count, c.t, got, c.want)
			}
		})
	}
}

func TestAddValidation(t *testing.T) {
	ok := Add{ID: uuid.NewString(), Nonce: "add-0001", ListID: uuid.NewString(), Name: "  Lena   Vogt ", PlusN: 1, ManagerPIN: "123456", At: at}
	p, problem := ok.parse(120, 10)
	if problem != "" || p.Name != "Lena Vogt" || p.id == uuid.Nil {
		t.Fatalf("valid add: %q %+v", problem, p)
	}
	cases := map[string]func(a *Add){
		"nonce":       func(a *Add) { a.Nonce = "x" },
		"id":          func(a *Add) { a.ID = uuid.Nil.String() },
		"list_id":     func(a *Add) { a.ListID = "list" },
		"name":        func(a *Add) { a.Name = strings.Repeat("a", 121) },
		"plus_n":      func(a *Add) { a.PlusN = 11 },
		"manager_pin": func(a *Add) { a.ManagerPIN = " " },
		"at":          func(a *Add) { a.At = "" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			a := ok
			mutate(&a)
			if _, problem := a.parse(120, 10); !strings.HasPrefix(problem, field) {
				t.Fatalf("problem %q, want %s", problem, field)
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	stamp := time.Date(2026, 10, 3, 23, 15, 1, 123456000, time.UTC)
	c := encodeCursor(stamp)
	got, err := decodeCursor(&c)
	if err != nil || !got.Equal(stamp) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	if got, err := decodeCursor(nil); got != nil || err != nil {
		t.Fatal("no cursor means everything")
	}
	for _, bad := range []string{"not base64!", "djIuMTIz", encodeCursor(time.Unix(0, 0))} {
		if _, err := decodeCursor(&bad); err == nil {
			t.Errorf("cursor %q accepted", bad)
		}
	}
}
