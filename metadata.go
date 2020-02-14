package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// To update, use:
// curl -s https://api.github.com/licenses | jq '.[].key'
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
	"mit":      "Expat",
	"mpl-2.0":  "MPL-2.0", // include in base-files >= 9.9
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

func findGitHubOwnerRepo(gopkg string) (string, error) {
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

func findGitHubRepo(gopkg string) (owner string, repo string, _ error) {
	ownerrepo, err := findGitHubOwnerRepo(gopkg)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(ownerrepo, "/")
	if got, want := len(parts), 2; got != want {
		return "", "", fmt.Errorf("invalid GitHub repo: %q does not follow owner/repo", repo)
	}
	return parts[0], parts[1], nil
}

func getLicenseForGopkg(gopkg string) (string, string, error) {
	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", "", err
	}

	rl, _, err := gitHub.Repositories.License(context.TODO(), owner, repo)
	if err != nil {
		return "", "", err
	}

	if deblicense, ok := githubLicenseToDebianLicense[rl.GetLicense().GetKey()]; ok {
		fulltext := debianLicenseText[deblicense]
		if fulltext == "" {
			fulltext = " TODO"
		}
		return deblicense, fulltext, nil
	}

	return "TODO", " TODO", nil
}

func getAuthorAndCopyrightForGopkg(gopkg string) (string, string, error) {
	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", "", err
	}

	rr, _, err := gitHub.Repositories.Get(context.TODO(), owner, repo)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(rr.GetOwner().GetURL()) == "" {
		return "", "", fmt.Errorf("Repository owner URL not present in API response")
	}

	ur, _, err := gitHub.Users.Get(context.TODO(), rr.GetOwner().GetLogin())
	if err != nil {
		return "", "", err
	}

	copyright := rr.CreatedAt.Format("2006") + " " + ur.GetName()
	if strings.HasPrefix(repo, "google/") {
		// As per https://opensource.google.com/docs/creating/, Google retains
		// the copyright for repositories underneath github.com/google/.
		copyright = rr.CreatedAt.Format("2006") + " Google Inc."
	}

	return ur.GetName(), copyright, nil
}

// getDescriptionForGopkg gets the package description from GitHub,
// intended for the synopsis or the short description in debian/control.
func getDescriptionForGopkg(gopkg string) (string, error) {
	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", err
	}

	rr, _, err := gitHub.Repositories.Get(context.TODO(), owner, repo)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(rr.GetDescription()), nil
}

func getHomepageForGopkg(gopkg string) string {
	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "TODO"
	}
	return "https://github.com/" + owner + "/" + repo
}
