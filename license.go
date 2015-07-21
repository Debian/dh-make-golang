package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type licensesReply struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Featured bool   `json:"featured"`
}

type repositoryReply struct {
	License licensesReply `json:"license"`
}

type licenseReply struct {
	Body string `json:"body"`
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
	//"mpl-2.0" (only 1.1 is in debian)
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
}

func getLicenseForGopkg(gopkg string) (string, string, error) {
	if !strings.HasPrefix(gopkg, "github.com/") {
		return "", "", nil
	}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+gopkg[len("github.com/"):], nil)
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
