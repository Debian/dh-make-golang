package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGetGolangBinaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		desc    string
		results []ftpMasterApiResult
		want    map[string]debianPackage
	}{
		{
			desc:    "no results",
			results: nil,
			want:    map[string]debianPackage{},
		},
		// It is unclear whether api.ftp-master.d.o would even return a result for a package whose
		// metadata field is set to the empty string (as of 2026-02-26 there are no such results), but
		// test the handling just in case.
		{
			desc: "empty",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{},
		},
		// It is unclear whether api.ftp-master.d.o would even return a result for a package whose
		// metadata field is set to a whitespace-only string (as of 2026-02-26 there are no such
		// results), but test the handling just in case.
		{
			desc: "whitespace-only",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: " \t\n\r",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{},
		},
		{
			desc: "leading whitespace",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: " \t\n\rexample.com/foo",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "trailing whitespace",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "example.com/foo \t\n\r",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "comma separation",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "example.com/foo,example.com/bar",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
				"example.com/bar": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "only commas",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: " ,\t,\n",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{},
		},
		{
			desc: "space around comma",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "example.com/foo ,\n\texample.com/bar",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
				"example.com/bar": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "leading comma",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: ",example.com/foo",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "trailing comma",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "example.com/foo,",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
		{
			desc: "internal extra comma",
			results: []ftpMasterApiResult{{
				Binary:        "golang-example-foo-dev",
				MetadataValue: "example.com/foo,,example.com/bar",
				Source:        "golang-example-foo",
			}},
			want: map[string]debianPackage{
				"example.com/foo": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
				"example.com/bar": {
					binary: "golang-example-foo-dev",
					source: "golang-example-foo",
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewEncoder(w).Encode(tc.results); err != nil {
					t.Fatal(err)
				}
			}))
			defer ts.Close()
			got, err := getGolangBinaries(withGolangBinariesUrl(ts.URL))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateComparable(debianPackage{})); diff != "" {
				t.Fatalf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
