package memgraph

import (
	"strings"
	"testing"
)

// TestDigitsOnly verifies that the digitsOnly function extracts only digits
// from a string, which is used for case number normalization.
func TestDigitsOnly(t *testing.T) {
	cases := map[string]string{
		// Formatted CNJ case number → bare digits
		"5083376-05.2014.4.04.7000": "50833760520144047000",
		// Bare case number → same
		"50833760520144047000": "50833760520144047000",
		// Partial case number → extracted digits
		"5083376": "5083376",
		// Non-digit string → empty
		"Apelação": "",
		// Mixed case number with spaces
		"5083376 - 05 . 2014 . 4 . 04 . 7000": "50833760520144047000",
		// Empty string
		"": "",
		// Only non-digits
		"abc-def.ghi": "",
	}
	for in, want := range cases {
		if got := digitsOnly(in); got != want {
			t.Errorf("digitsOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSearchProceedingsQuery verifies that the proceedings search query
// includes the correct fields and logic for case number normalization.
func TestSearchProceedingsQuery(t *testing.T) {
	query := searchProceedingsQuery()

	// Verify the query includes the required fields
	requiredFields := []string{
		"node:LegalProceeding",
		"node.case_number",
		"node.class_name",
		"node.court",
		"$q_digits",
		"$q",
	}
	for _, field := range requiredFields {
		if !strings.Contains(query, field) {
			t.Errorf("searchProceedingsQuery missing %q: %s", field, query)
		}
	}

	// Verify case number normalization: removing hyphens and dots
	if !strings.Contains(query, "replace(") {
		t.Errorf("searchProceedingsQuery should use replace() for case number normalization: %s", query)
	}

	// Verify the query uses foldExpr for accent/case folding
	if !strings.Contains(query, "toLower(") {
		t.Errorf("searchProceedingsQuery should use foldExpr (which calls toLower) for folding: %s", query)
	}

	// Verify LIMIT
	if !strings.Contains(query, "LIMIT 20") {
		t.Errorf("searchProceedingsQuery should include LIMIT 20: %s", query)
	}
}

// TestSearchAllQueryIncludesProceedingsType verifies that the all-types search
// query includes legal proceedings with the same normalization logic.
func TestSearchAllQueryIncludesProceedingsType(t *testing.T) {
	query := searchAllQuery()

	// Verify LegalProceeding is included
	if !strings.Contains(query, "node:LegalProceeding") {
		t.Errorf("searchAllQuery should include node:LegalProceeding: %s", query)
	}

	// Verify case number normalization is included in searchAllQuery
	if !strings.Contains(query, "replace(replace(node.case_number") {
		t.Errorf("searchAllQuery should include case number normalization: %s", query)
	}

	// Verify the query uses $q_digits parameter
	if !strings.Contains(query, "$q_digits") {
		t.Errorf("searchAllQuery should use $q_digits parameter: %s", query)
	}
}

// TestCaseNumberNormalizationLogic verifies the logic for case number matching:
// formatted numbers (with hyphens/dots) should match bare digit numbers.
func TestCaseNumberNormalizationLogic(t *testing.T) {
	cases := []struct {
		storedNumber string
		queryNumber  string
		shouldMatch  bool
		description  string
	}{
		// Exact match (bare digits)
		{"50833760520144047000", "50833760520144047000", true, "exact bare digit match"},
		// Formatted query → bare stored
		{"50833760520144047000", "5083376-05.2014.4.04.7000", true, "formatted query matches bare stored"},
		// Partial match (first 7 digits)
		{"50833760520144047000", "5083376", true, "partial digit query matches"},
		// Partial formatted match
		{"50833760520144047000", "5083376-05", true, "partial formatted query matches"},
		// Different case numbers should not match
		{"50833760520144047000", "12345678901234567890", false, "different case numbers don't match"},
	}

	for _, tc := range cases {
		// This verifies the normalization logic by checking if the digit-only versions
		// match as substrings
		storedDigits := digitsOnly(tc.storedNumber)
		queryDigits := digitsOnly(tc.queryNumber)
		matches := strings.Contains(storedDigits, queryDigits) || strings.Contains(queryDigits, storedDigits)

		if matches != tc.shouldMatch {
			t.Errorf("Case number normalization for %s: stored=%q query=%q, expected=%v, got=%v",
				tc.description, storedDigits, queryDigits, tc.shouldMatch, matches)
		}
	}
}

// TestSearchQueryParameterNormalization verifies that the search query parameter
// is properly prepared with both folded text and digits-only versions.
func TestSearchQueryParameterNormalization(t *testing.T) {
	cases := []struct {
		input        string
		expectedFold string
		expectedDig  string
		description  string
	}{
		// Non-digit query
		{"Apelação", "apelacao", "", "accented non-digit query"},
		// Bare case number
		{"50833760520144047000", "50833760520144047000", "50833760520144047000", "bare case number"},
		// Formatted case number
		{"5083376-05.2014.4.04.7000", "5083376-05.2014.4.04.7000", "50833760520144047000", "formatted case number"},
		// Partial case number
		{"5083376", "5083376", "5083376", "partial case number"},
		// Mixed content
		{"Ação 5083", "acao 5083", "5083", "mixed action and partial number"},
	}

	for _, tc := range cases {
		fold := foldQuery(tc.input)
		dig := digitsOnly(fold)

		if fold != tc.expectedFold {
			t.Errorf("foldQuery(%q) for %s: expected %q, got %q", tc.input, tc.description, tc.expectedFold, fold)
		}
		if dig != tc.expectedDig {
			t.Errorf("digitsOnly(foldQuery(%q)) for %s: expected %q, got %q", tc.input, tc.description, tc.expectedDig, dig)
		}
	}
}
