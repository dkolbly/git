package gitcamli

import (
	"camlistore.org/pkg/client"
	"github.com/dkolbly/git"
	"github.com/dkolbly/logging"
)

var log = logging.New("gitcamli")

// A *Client satisfies the git.Store interface, so you can
// access a git-structured dataset in camlistore by creating
// a git.Repo and adding one of these
type Client struct {
	repo     *git.Git
	camli    *client.Client
	repoName string
	// TODO mutex
	cache map[git.Ptr]*CamliGitObject
}

// create a new client for accessing the given named repository
// within the connected camlistore
func New(repo *git.Git, name string, conn *client.Client) *Client {
	c := &Client{
		repo:     repo,
		repoName: name,
		camli:    conn,
		cache:    make(map[git.Ptr]*CamliGitObject),
	}
	repo.AddStore(c)
	return c
}

func (cc *Client) EnumerateTo(ch chan<- git.Ptr) {

}
