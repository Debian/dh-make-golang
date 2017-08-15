package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type licensesReply struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Featured bool   `json:"featured"`
}

type ownerReply struct {
	URL string `json:"url"`
}

type repositoryReply struct {
	License     licensesReply `json:"license"`
	CreatedAt   string        `json:"created_at"`
	Description string        `json:"description"`
	Owner       ownerReply    `json:"owner"`
}

type licenseReply struct {
	Body string `json:"body"`
}

type usersReply struct {
	Name string `json:"name"`
}

// To update, use:
// curl -s -H 'Accept: application/vnd.github.drax-preview+json' https://api.github.com/licenses | jq '.[].key'
// then compare with https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/#license-specification
var githubLicenseToDebianLicense = map[string]string{
	//"agpl-3.0" (not in debian?)
	"apache-2.0":   "Apache-2.0",
	"artistic-2.0": "Artistic-2.0",
	"bsd-2-clause": "BSD-2-clause",
	"bsd-3-clause": "BSD-3-clause",
	"cc0-1.0":      "CC0-1.0",
	//"epl-1.0" (eclipse public license)
	"gpl-2.0":  "GPL-2.0", // TODO: is this GPL-2.0+?
	"gpl-3.0":  "GPL-3.0",
	"isc":      "ISC",
	"lgpl-2.1": "LGPL-2.1",
	"lgpl-3.0": "LGPL-3.0",
	//"mit" - expat?
	"mpl-2.0": "MPL-2.0", // include in base-files >= 9.9
	//"unlicense" (not in debian)
}

var debianLicenseText = map[string]string{
	"Apache-2.0": ` Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
 .
 http://www.apache.org/licenses/LICENSE-2.0
 .
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
 .
 On Debian systems, the complete text of the Apache version 2.0 license
 can be found in "/usr/share/common-licenses/Apache-2.0".
`,
	"MPL-2.0": ` This Source Code Form is subject to the terms of the Mozilla Public
 License, v. 2.0. If a copy of the MPL was not distributed with this
 file, You can obtain one at http://mozilla.org/MPL/2.0/.
 .
 On Debian systems, the complete text of the MPL-2.0 license can be found
 in "/usr/share/common-licenses/MPL-2.0".
`,
}

var githubRegexp = regexp.MustCompile(`github\.com/([^/]+/[^/]+)`)

func findGitHubRepo(gopkg string) (string, error) {
	if strings.HasPrefix(gopkg, "github.com/") {
		return strings.TrimPrefix(gopkg, "github.com/"), nil
	}
	resp, err := http.Get("https://" + gopkg + "?go-get=1")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return "", fmt.Errorf("%q is not on GitHub", gopkg)
		}
		token := z.Token()
		if token.Data != "meta" {
			continue
		}
		var meta struct {
			name, content string
		}
		for _, attr := range token.Attr {
			if attr.Key == "name" {
				meta.name = attr.Val
			}
			if attr.Key == "content" {
				meta.content = attr.Val
			}
		}

		match := func(name string, length int) string {
			if f := strings.Fields(meta.content); meta.name == name && len(f) == length {
				if f[0] != gopkg {
					return ""
				}
				if repoMatch := githubRegexp.FindStringSubmatch(f[2]); repoMatch != nil {
					return repoMatch[1]
				}
			}
			return ""
		}
		if repo := match("go-import", 3); repo != "" {
			return repo, nil
		}
		if repo := match("go-source", 4); repo != "" {
			return repo, nil
		}
	}
}

func getLicenseForGopkg(gopkg string) (string, string, error) {
	repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", "", err
	}
	// TODO: cache this reply
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repo, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github.drax-preview+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var r repositoryReply
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}
	if deblicense, ok := githubLicenseToDebianLicense[r.License.Key]; ok {
		fulltext := debianLicenseText[deblicense]
		if fulltext == "" {
			fulltext = "TODO"
		}
		return deblicense, fulltext, nil
	} else {
		return "TODO", "TODO", nil
	}
}

func getAuthorAndCopyrightForGopkg(gopkg string) (string, string, error) {
	repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", "", err
	}
	resp, err := http.Get("https://api.github.com/repos/" + repo)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var rr repositoryReply
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return "", "", err
	}

	creation, err := time.Parse("2006-01-02T15:04:05Z", rr.CreatedAt)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(rr.Owner.URL) == "" {
		return "", "", fmt.Errorf("Repository owner URL not present in API response")
	}

	resp, err = http.Get(rr.Owner.URL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var ur usersReply
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return "", "", err
	}

	copyright := creation.Format("2006") + " " + ur.Name
	if strings.HasPrefix(repo, "google/") {
		// As per https://opensource.google.com/docs/creating/, Google retains
		// the copyright for repositories underneath github.com/google/.
		copyright = creation.Format("2006") + " Google Inc."
	}

	return ur.Name, copyright, nil
}

func getDescriptionForGopkg(gopkg string) (string, error) {
	repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", err
	}
	resp, err := http.Get("https://api.github.com/repos/" + repo)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rr repositoryReply
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return "", err
	}

	return strings.TrimSpace(rr.Description), nil
}

func getHomepageForGopkg(gopkg string) string {
	repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "TODO"
	}
	return "https://github.com/" + repo
}
