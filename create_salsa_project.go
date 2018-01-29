package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

const (
	salsaGroupID = 2638
	salsaApiUrl  = "https://salsa.debian.org/api/v4"
)

type gitlabProject struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	HttpUrlToRepo string `json:"http_url_to_repo"`
}

func execCreateSalsaProject(args []string) {
	projectName := mustGetProjectName(args)
	token := mustGetSalsaToken()

	gitlabProject, err := createSalsaProject(projectName, token)
	if err != nil {
		log.Fatalf("Could not create %s on salsa: %s\n", projectName, err)
	}

	fmt.Printf("Project %s was created on salsa.debian.org: %s\n",
		gitlabProject.Name,
		gitlabProject.HttpUrlToRepo)

	if err := createSalsaWebhook(gitlabProject.ID,
		"https://webhook.salsa.debian.org/tagpending/"+gitlabProject.Name,
		token); err != nil {
		log.Fatalf("Could not create webhook on salsa project: %s\n", err)
	}

	for _, branch := range []string{"master", "debian/*", "upstream", "upstream/*", "pristine-tar"} {
		if err := protectSalsaProjectBranch(gitlabProject.ID, branch, token); err != nil {
			log.Printf("Could not protect branch %s: %s\n", branch, err)
		}
	}

}

func postFormToSalsaApi(path string, data url.Values, token string) (*http.Response, error) {
	postUrl := salsaApiUrl + path
	data.Add("private_token", token)
	return http.PostForm(postUrl, data)
}

func protectSalsaProjectBranch(projectId int, branch, token string) error {
	postPath := "/projects/" + strconv.Itoa(projectId) + "/protected_branches"

	response, err := postFormToSalsaApi(postPath,
		url.Values{
			"name": {branch},
		},
		token)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(response.Body)

		return fmt.Errorf("http status %d: %s",
			response.StatusCode,
			responseData)
	}

	return nil
}

func createSalsaWebhook(projectId int, webhookUrl, token string) error {
	postPath := "/projects/" + strconv.Itoa(projectId) + "/hooks"

	response, err := postFormToSalsaApi(postPath,
		url.Values{
			"url":         {webhookUrl},
			"push_events": {"true"},
		},
		token)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(response.Body)

		return fmt.Errorf("http status %d: %s",
			response.StatusCode,
			responseData)
	}

	return nil
}

func mustGetProjectName(args []string) string {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	if fs.NArg() != 1 {
		log.Printf("Usage: %s create-salsa-project <project-name>\n", os.Args[0])
		log.Fatalf("Example: %s create-salsa-project golang-github-mattn-go-sqlite3\n", os.Args[0])
	}

	projectName := fs.Arg(0)

	return projectName
}

func mustGetSalsaToken() string {
	token := os.Getenv("SALSA_TOKEN")
	if token == "" {
		log.Printf("Please set the SALSA_TOKEN environment variable.\n")
		log.Fatalf("Obtain it from the following page: https://salsa.debian.org/profile/personal_access_tokens\n")
	}
	return token
}

func createSalsaProject(projectName, token string) (*gitlabProject, error) {
	response, err := postFormToSalsaApi("/projects",
		url.Values{
			"private_token": {token},
			"path":          {projectName},
			"namespace_id":  {strconv.Itoa(salsaGroupID)},
			"description":   {fmt.Sprintf("Debian packaging for %s", projectName)},
			"visibility":    {"public"},
		},
		token)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusCreated {
		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}

		return nil, errors.New(fmt.Sprintf("http status %d: %s", response.StatusCode, responseData))
	}

	var project gitlabProject

	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		return nil, err
	}

	return &project, err
}
