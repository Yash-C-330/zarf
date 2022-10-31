package git

import (
	"context"
	"errors"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/go-git/go-git/v5"
)

// clone performs a `git clone` of a given repo.
func (g *Git) clone(gitDirectory string, gitURL string, onlyFetchRef bool) (*git.Repository, error) {
	cloneOptions := &git.CloneOptions{
		URL:        gitURL,
		Progress:   g.Spinner,
		RemoteName: onlineRemoteName,
	}

	if onlyFetchRef {
		cloneOptions.Tags = git.NoTags
	}

	gitCred := g.FindAuthForHost(gitURL)

	// Gracefully handle no git creds on the system (like our CI/CD)
	if gitCred.Auth.Username != "" {
		cloneOptions.Auth = &gitCred.Auth
	}

	// Clone the given repo
	repo, err := git.PlainClone(gitDirectory, false, cloneOptions)

	if errors.Is(err, git.ErrRepositoryAlreadyExists) {
		repo, err = git.PlainOpen(gitDirectory)

		if err != nil {
			return nil, err
		}

		return repo, git.ErrRepositoryAlreadyExists
	} else if err != nil {
		g.Spinner.Debugf("Failed to clone repo: %s", err)
		message.Infof("Falling back to host git for %s", gitURL)

		// If we can't clone with go-git, fallback to the host clone
		// Only support "all tags" due to the azure clone url format including a username
		cmdArgs := []string{"clone", "--origin", onlineRemoteName, gitURL, gitDirectory}

		if onlyFetchRef {
			cmdArgs = append(cmdArgs, "--no-tags")
		}

		stdOut, stdErr, err := utils.ExecCommandWithContext(context.TODO(), false, "git", cmdArgs...)
		g.Spinner.Updatef(stdOut)
		g.Spinner.Debugf(stdErr)

		if err != nil {
			return nil, err
		}

		return git.PlainOpen(gitDirectory)
	} else {
		return repo, nil
	}
}
