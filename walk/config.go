/* Copyright 2018 The Bazel Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package walk

import (
	"flag"
	"log"
	"os"
	"path"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bmatcuk/doublestar"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"

	gzflag "github.com/bazelbuild/bazel-gazelle/flag"
)

// TODO(#472): store location information to validate each exclude. They
// may be set in one directory and used in another. Excludes work on
// declared generated files, so we can't just stat.

type walkConfig struct {
	excludes         []string
	ignore           bool
	follow           []string
	gitignoreMatcher gitignore.Matcher
}

const walkName = "_walk"

func getWalkConfig(c *config.Config) *walkConfig {
	return c.Exts[walkName].(*walkConfig)
}

func (wc *walkConfig) isExcluded(rel, base string) bool {
	if base == ".git" {
		return true
	}
	f := path.Join(rel, base)
	if wc.gitignoreMatcher != nil {
		path := strings.Split(f, string(os.PathSeparator))
		match := wc.gitignoreMatcher.Match(path, false)
		if match {
			return true
		}
	}
	for _, x := range wc.excludes {
		matched, err := doublestar.Match(x, f)
		if err != nil {
			// doublestar.Match returns only one possible error, and only if the
			// pattern is not valid. During the configuration of the walker (see
			// Configure below), we discard any invalid pattern and thus an error
			// here should not be possible.
			log.Panicf("error during doublestar.Match. This should not happen, please file an issue https://github.com/bazelbuild/bazel-gazelle/issues/new: %s", err)
		}
		if matched {
			return true
		}
	}
	return false
}

type Configurer struct{}

func (_ *Configurer) RegisterFlags(fs *flag.FlagSet, cmd string, c *config.Config) {
	wc := &walkConfig{}
	c.Exts[walkName] = wc
	fs.Var(&gzflag.MultiFlag{Values: &wc.excludes}, "exclude", "pattern that should be ignored (may be repeated)")
}

func (_ *Configurer) CheckFlags(fs *flag.FlagSet, c *config.Config) error { return nil }

func (_ *Configurer) KnownDirectives() []string {
	return []string{"exclude", "follow", "ignore", "use_gitignore"}
}

func (_ *Configurer) Configure(c *config.Config, rel string, f *rule.File) {
	wc := getWalkConfig(c)
	wcCopy := &walkConfig{}
	*wcCopy = *wc
	wcCopy.ignore = false

	if f != nil {
		for _, d := range f.Directives {
			switch d.Key {
			case "exclude":
				if err := checkPathMatchPattern(path.Join(rel, d.Value)); err != nil {
					log.Printf("the exclusion pattern is not valid %q: %s", path.Join(rel, d.Value), err)
					continue
				}
				wcCopy.excludes = append(wcCopy.excludes, path.Join(rel, d.Value))
			case "follow":
				wcCopy.follow = append(wcCopy.follow, path.Join(rel, d.Value))
			case "ignore":
				wcCopy.ignore = true
			case "use_gitignore":
				if wcCopy.gitignoreMatcher == nil {
					wcCopy.gitignoreMatcher = createGitignoreMatcher(c.RepoRoot)
				}
			}
		}
	}

	c.Exts[walkName] = wcCopy
}

func checkPathMatchPattern(pattern string) error {
	_, err := doublestar.Match(pattern, "x")
	return err
}

func createGitignoreMatcher(rootDir string) gitignore.Matcher {
	fs := osfs.New(rootDir)
	ps, err := gitignore.ReadPatterns(fs, []string{})
	if err != nil {
		log.Print("failed to read .gitignore")
	}
	matcher := gitignore.NewMatcher(ps)
	return matcher
}
