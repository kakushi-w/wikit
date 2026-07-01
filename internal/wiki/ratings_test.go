package wiki

import "testing"

func TestParseRatingRows(t *testing.T) {
	// Mimics a ListPages body: a pager block (whose "page 1 of 2" text must not
	// leak into the last row) followed by the sentinel-wrapped rows. Includes a
	// +/- score, zero, a negative, a five-star decimal, and a blank (unrated).
	body := `<div class="list-pages-box">
<div class="pager"><span class="pager-no">page 1 of 2</span></div>
<p>@@ROW@@wikit-cli@@F@@1
@@ROW@@file:spaceword@@F@@0
@@ROW@@neg-page@@F@@-3
@@ROW@@star-page@@F@@4.5
@@ROW@@unrated@@F@@</p>
</div>`

	table := map[string]float64{}
	parseRatingRows(body, table)

	want := map[string]float64{
		"wikit-cli":      1,
		"file:spaceword": 0,
		"neg-page":       -3,
		"star-page":      4.5,
	}
	if len(table) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(table), len(want), table)
	}
	for k, v := range want {
		if got, ok := table[k]; !ok || got != v {
			t.Errorf("%s: got %v (present=%v), want %v", k, got, ok, v)
		}
	}
	if _, ok := table["unrated"]; ok {
		t.Errorf("blank rating should be skipped, not stored")
	}
}

func TestFormatRating(t *testing.T) {
	cases := map[float64]string{
		5:    "5",   // whole numbers stay integer-formatted (byte-compatible)
		0:    "0",
		-3:   "-3",
		4.5:  "4.5",
		3.25: "3.25",
	}
	for in, want := range cases {
		if got := formatRating(in); got != want {
			t.Errorf("formatRating(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestRatingsEqual(t *testing.T) {
	five := 5.0
	if !ratingsEqual(&five, 5) {
		t.Error("5 should equal 5")
	}
	if ratingsEqual(&five, 6) {
		t.Error("5 should not equal 6")
	}
	if ratingsEqual(nil, 0) {
		t.Error("absent rating should count as different so it gets written")
	}
}
