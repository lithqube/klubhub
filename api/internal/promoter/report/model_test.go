package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestBucketStartAlignsToLocalQuarterHours(t *testing.T) {
	berlin := zone(t, "Europe/Berlin")
	kathmandu := zone(t, "Asia/Kathmandu") // +05:45
	odd := time.FixedZone("LMT+0020", 20*60)
	for _, tc := range []struct {
		name string
		at   string
		loc  *time.Location
		want string
	}{
		{"on the boundary", "2026-07-04T22:15:00Z", berlin, "2026-07-04T22:15:00Z"},
		{"inside a bucket, seconds dropped", "2026-07-04T22:29:59.9Z", berlin, "2026-07-04T22:15:00Z"},
		{"last CEST quarter of a fall-back night", "2026-10-25T00:59:00Z", berlin, "2026-10-25T00:45:00Z"},
		{"repeated 02:00 hour after fall-back is its own bucket", "2026-10-25T01:05:00Z", berlin, "2026-10-25T01:00:00Z"},
		{"+05:45 zone", "2026-07-04T10:07:00Z", kathmandu, "2026-07-04T10:00:00Z"},
		{"+05:45 zone next quarter", "2026-07-04T10:20:00Z", kathmandu, "2026-07-04T10:15:00Z"},
		{"offset not a multiple of 15 minutes aligns locally", "2026-07-04T10:07:00Z", odd, "2026-07-04T09:55:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bucketStart(utc(tc.at), tc.loc)
			if want := utc(tc.want); !got.Equal(want) {
				t.Fatalf("bucketStart(%s) = %s, want %s", tc.at, got, want)
			}
			if lt := got.In(tc.loc); lt.Minute()%15 != 0 || lt.Second() != 0 || lt.Nanosecond() != 0 {
				t.Fatalf("not aligned locally: %s", lt)
			}
		})
	}
}

func TestBuildCurve(t *testing.T) {
	berlin := zone(t, "Europe/Berlin")
	lordHowe := zone(t, "Australia/Lord_Howe") // 30-minute DST shift
	type want struct {
		local                 string // "15:04" in the zone
		in, out, walkups, occ int
	}
	for _, tc := range []struct {
		name   string
		loc    *time.Location
		points []point
		want   []want
	}{
		{name: "no activity", loc: berlin, points: nil, want: []want{}},
		{
			name: "empty buckets between activity are kept",
			loc:  berlin,
			points: []point{
				{At: utc("2026-07-04T21:03:00Z"), In: 3},
				{At: utc("2026-07-04T21:10:00Z"), In: 1, Walkups: 2},
				{At: utc("2026-07-04T21:50:00Z"), Out: 2},
			},
			want: []want{
				{"23:00", 4, 0, 2, 6},
				{"23:15", 0, 0, 0, 6},
				{"23:30", 0, 0, 0, 6},
				{"23:45", 0, 2, 0, 4},
			},
		},
		{
			name: "fall-back night: 02:00-03:00 happens twice",
			loc:  berlin,
			points: []point{
				{At: utc("2026-10-25T00:40:00Z"), In: 5},  // 02:40 CEST
				{At: utc("2026-10-25T00:50:00Z"), In: 1},  // 02:50 CEST
				{At: utc("2026-10-25T01:05:00Z"), Out: 2}, // 02:05 CET
				{At: utc("2026-10-25T01:20:00Z"), In: 4},  // 02:20 CET
			},
			want: []want{
				{"02:30", 5, 0, 0, 5},
				{"02:45", 1, 0, 0, 6},
				{"02:00", 0, 2, 0, 4},
				{"02:15", 4, 0, 0, 8},
			},
		},
		{
			name: "spring-forward night: no buckets for the missing hour",
			loc:  berlin,
			points: []point{
				{At: utc("2026-03-29T00:55:00Z"), In: 2}, // 01:55 CET
				{At: utc("2026-03-29T01:05:00Z"), In: 1}, // 03:05 CEST
			},
			want: []want{
				{"01:45", 2, 0, 0, 2},
				{"03:00", 1, 0, 0, 3},
			},
		},
		{
			name: "30-minute DST shift stays on local quarters",
			loc:  lordHowe,
			points: []point{
				{At: utc("2026-10-03T15:20:00Z"), In: 1}, // 01:50 +10:30
				{At: utc("2026-10-03T15:31:00Z"), In: 1}, // 02:31 +11:00
			},
			want: []want{
				{"01:45", 1, 0, 0, 1},
				{"02:30", 1, 0, 0, 2},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCurve(tc.points, tc.loc)
			if got == nil || len(got) != len(tc.want) {
				t.Fatalf("got %d buckets, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				b := got[i]
				if l := b.BucketStart.In(tc.loc).Format("15:04"); l != w.local || b.In != w.in || b.Out != w.out || b.Walkups != w.walkups || b.Occupancy != w.occ {
					t.Errorf("bucket %d = %s %+v, want %+v", i, l, b, w)
				}
				if i > 0 && b.BucketStart.Sub(got[i-1].BucketStart) != BucketSize {
					t.Errorf("bucket %d is not 15 minutes after the previous one", i)
				}
				if b.BucketStart.Location() != time.UTC {
					t.Errorf("bucket starts are UTC in JSON")
				}
			}
		})
	}
}

func TestPeak(t *testing.T) {
	a, b, c := utc("2026-07-04T21:00:00Z"), utc("2026-07-04T21:15:00Z"), utc("2026-07-04T21:30:00Z")
	for _, tc := range []struct {
		name   string
		curve  []Bucket
		want   int
		wantAt *time.Time
	}{
		{"empty", nil, 0, nil},
		{"never above zero", []Bucket{{BucketStart: a, Occupancy: 0}, {BucketStart: b, Occupancy: -2}}, 0, nil},
		{"first bucket reaching the max", []Bucket{{BucketStart: a, Occupancy: 4}, {BucketStart: b, Occupancy: 9}, {BucketStart: c, Occupancy: 9}}, 9, &b},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, at := peak(tc.curve)
			if got != tc.want || (at == nil) != (tc.wantAt == nil) || (at != nil && !at.Equal(*tc.wantAt)) {
				t.Fatalf("peak = %d %v, want %d %v", got, at, tc.want, tc.wantAt)
			}
		})
	}
}

func TestClampPoints(t *testing.T) {
	from, to := utc("2026-07-04T20:00:00Z"), utc("2026-07-05T08:00:00Z")
	got := clampPoints([]point{{At: utc("1970-01-01T00:00:00Z"), In: 1}, {At: utc("2026-07-04T23:00:00Z"), In: 2}, {At: utc("2030-01-01T00:00:00Z"), Out: 1}}, from, to)
	if !got[0].At.Equal(from) || !got[1].At.Equal(utc("2026-07-04T23:00:00Z")) || !got[2].At.Equal(to) || got[0].In != 1 || got[2].Out != 1 {
		t.Fatalf("clamp: %+v", got)
	}
}

func TestNoShowRate(t *testing.T) {
	for _, tc := range []struct {
		going, arrived int
		want           *float64
	}{
		{0, 0, nil},
		{0, 3, nil}, // arrivals without anyone going: still undefined
		{4, 4, ptr(0)},
		{4, 1, ptr(0.75)},
		{3, 1, ptr(0.6667)},
	} {
		got := noShowRate(tc.going, tc.arrived)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("noShowRate(%d, %d) = %v, want %v", tc.going, tc.arrived, deref(got), deref(tc.want))
		}
	}
}

func ptr(f float64) *float64 { return &f }

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func TestTallyGuest(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   guestFacts
		want guestTally
	}{
		{"going no-show", guestFacts{"going", 2, 0}, guestTally{Going: 1, HeadsExpected: 3, PlusAllowed: 2}},
		{"going alone", guestFacts{"going", 2, 1}, guestTally{Going: 1, Arrived: 1, GoingArrived: 1, HeadsExpected: 3, HeadsAdmitted: 1, PlusAllowed: 2}},
		{"partial +N", guestFacts{"going", 3, 2}, guestTally{Going: 1, Arrived: 1, GoingArrived: 1, HeadsExpected: 4, HeadsAdmitted: 2, PlusAllowed: 3, PlusUsed: 1}},
		{"over-admitted: +1s capped at plus_n", guestFacts{"going", 1, 4}, guestTally{Going: 1, Arrived: 1, GoingArrived: 1, HeadsExpected: 2, HeadsAdmitted: 4, PlusAllowed: 1, PlusUsed: 1}},
		{"pending guest admitted anyway", guestFacts{"pending", 1, 2}, guestTally{Arrived: 1, HeadsAdmitted: 2, PlusUsed: 1}},
		{"declined, not seen", guestFacts{"declined", 1, 0}, guestTally{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tallyGuest(tc.in); got != tc.want {
				t.Fatalf("tallyGuest(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRollup(t *testing.T) {
	l1, l2, a1, a2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	one := func(fs ...guestFacts) guestTally {
		var t guestTally
		for _, f := range fs {
			t.add(tallyGuest(f))
		}
		return t
	}
	groups := map[groupKey]guestTally{
		{List: l1}:                 one(guestFacts{"going", 1, 2}),
		{List: l1, Allocation: a1}: one(guestFacts{"going", 0, 1}, guestFacts{"going", 2, 0}),
	}
	total, lists, subs := rollup(groups, []ListRow{{ListID: l1}, {ListID: l2}}, []SubmitterRow{{AllocationID: a1}, {AllocationID: a2}})
	if total.Going != 3 || total.Arrived != 2 || total.HeadsExpected != 2+1+3 || total.HeadsAdmitted != 3 || total.PlusUsed != 1 || total.PlusAllowed != 3 {
		t.Fatalf("total: %+v", total)
	}
	if lists[0].Going != 3 || lists[0].Arrived != 2 || *lists[0].NoShowRate != 0.3333 || lists[1].Going != 0 || lists[1].NoShowRate != nil {
		t.Fatalf("lists: %+v", lists)
	}
	if subs[0].Going != 2 || subs[0].Arrived != 1 || *subs[0].NoShowRate != 0.5 || subs[0].HeadsAdmitted != 1 || subs[1].NoShowRate != nil {
		t.Fatalf("submitters: %+v", subs)
	}
}

func TestWriteListBack(t *testing.T) {
	berlin := zone(t, "Europe/Berlin")
	first := utc("2026-07-04T22:07:31Z")
	var buf bytes.Buffer
	err := WriteListBack(&buf, []ListBackRow{
		{Name: "=HYPERLINK(\"http://x\")", PlusN: 1, Status: "going", HeadsAdmitted: 2, FirstIn: &first},
		{Name: "Lena, \"DJ\" Vogt", PlusN: 0, Status: "pending"},
		{Name: "@cmd", PlusN: 0, Status: "declined"},
	}, berlin)
	if err != nil {
		t.Fatal(err)
	}
	want := "name,plus_n,status,arrived,heads_admitted,first_in_local\n" +
		"\"'=HYPERLINK(\"\"http://x\"\")\",1,going,yes,2,2026-07-05 00:07\n" +
		"\"Lena, \"\"DJ\"\" Vogt\",0,pending,no,0,\n" +
		"'@cmd,0,declined,no,0,\n"
	if got := buf.String(); got != want {
		t.Fatalf("csv:\n%s\nwant:\n%s", got, want)
	}
}

func TestFilename(t *testing.T) {
	id := uuid.MustParse("0190f1d2-7c1a-7a00-9f00-00000000beef")
	for _, tc := range []struct{ label, want string }{
		{"Ben Klock", "klubnacht-list-back-ben-klock.csv"},
		{"Ø \"Røyksopp\"; rm -rf", "klubnacht-list-back-o-royksopp-rm-rf.csv"},
		{"💿", "klubnacht-list-back-0190f1d2.csv"},
	} {
		if got := filename("klubnacht", tc.label, id); got != tc.want {
			t.Errorf("filename(%q) = %q, want %q", tc.label, got, tc.want)
		}
		if strings.ContainsAny(filename("klubnacht", tc.label, id), "\" ;\r\n") {
			t.Errorf("unsafe filename for %q", tc.label)
		}
	}
}
