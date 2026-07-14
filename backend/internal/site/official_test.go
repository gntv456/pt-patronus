package site

import "testing"

func TestIsOfficial(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		labels []string
		want   bool
	}{
		{"label 官方", "Movie.2023", []string{"官方"}, true},
		{"label 官种", "Movie.2023", []string{"Free", "官种"}, true},
		{"title 官组", "[官组]Movie", nil, true},
		{"title 官方发布", "官方发布.Movie", nil, true},
		{"非官种", "普通种子", []string{"Free"}, false},
		{"空", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOfficial(tc.title, tc.labels); got != tc.want {
				t.Errorf("IsOfficial(%q,%v) = %v, want %v", tc.title, tc.labels, got, tc.want)
			}
		})
	}
}
