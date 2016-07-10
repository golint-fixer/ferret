/*
 * Ferret
 * Copyright (c) 2016 Yieldbot, Inc.
 * For the full copyright and license information, please view the LICENSE.txt file.
 */

// Package github implements AnswerHub provider
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// Provider represents the provider
type Provider struct {
	name       string
	title      string
	url        string
	token      string
	searchUser string
}

// Register registers the provider
func Register(f func(provider interface{}) error) {
	// Init the provider
	var p = Provider{
		name:       "github",
		title:      "Github",
		url:        strings.TrimSuffix(os.Getenv("FERRET_GITHUB_URL"), "/"),
		token:      os.Getenv("FERRET_GITHUB_TOKEN"),
		searchUser: os.Getenv("FERRET_GITHUB_SEARCH_USER"),
	}

	// Register the provider
	if err := f(&p); err != nil {
		panic(err)
	}
}

// SearchResult represent the structure of the search result
type SearchResult struct {
	TotalCount        int        `json:"total_count"`
	IncompleteResults bool       `json:"incomplete_results"`
	Items             []*SRItems `json:"items"`
}

// SRItems represent the structure of the search result items
type SRItems struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	HTMLUrl     string         `json:"html_url"`
	Repository  *SRIRepository `json:"repository"`
	TextMatches []*SRITMatches `json:"text_matches"`
}

// SRIRepository represent the structure of the search result items repository
type SRIRepository struct {
	Fullname    string `json:"full_name"`
	Description string `json:"description"`
}

// SRITMatches represent the structure of the search result items text matches field
type SRITMatches struct {
	Fragment string `json:"fragment"`
}

// Search makes a search
func (provider *Provider) Search(ctx context.Context, args map[string]interface{}) ([]map[string]interface{}, error) {

	results := []map[string]interface{}{}
	page, ok := args["page"].(int)
	if page < 1 || !ok {
		page = 1
	}
	keyword, ok := args["keyword"].(string)

	var u = fmt.Sprintf("%s/search/code?page=%d&per_page=10&q=%s", provider.url, page, url.QueryEscape(keyword))
	if provider.searchUser != "" {
		u += fmt.Sprintf("+user:%s", url.QueryEscape(provider.searchUser))
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.New("failed to prepare request. Error: " + err.Error())
	}
	if provider.token != "" {
		req.Header.Set("Authorization", "token "+provider.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3.text-match+json")

	res, err := ctxhttp.Do(ctx, nil, req)
	if err != nil {
		return nil, errors.New("failed to fetch data. Error: " + err.Error())
	} else if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.New("bad response: " + fmt.Sprintf("%d", res.StatusCode))
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var sr SearchResult
	if err = json.Unmarshal(data, &sr); err != nil {
		return nil, errors.New("failed to unmarshal JSON data. Error: " + err.Error())
	}
	for _, v := range sr.Items {
		var d string
		if len(v.TextMatches) > 0 {
			for _, tm := range v.TextMatches {
				d = d + tm.Fragment + "..."
			}
		} else {
			d = v.Repository.Description
		}
		d = strings.TrimSpace(strings.TrimSuffix(d, "..."))
		if len(d) > 255 {
			d = d[0:252] + "..."
		}
		ri := map[string]interface{}{
			"Link":        v.HTMLUrl,
			"Title":       fmt.Sprintf("%s/%s", v.Repository.Fullname, strings.TrimPrefix(v.Path, "/")),
			"Description": d,
		}
		results = append(results, ri)
	}

	return results, err
}
