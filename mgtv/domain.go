// Package mgtv: kit Domain registration for mgtv.com (Mango TV / 芒果TV).
//
// Import this package blank in a multi-domain host to enable the mgtv:// driver:
//
//	import _ "github.com/tamnd/mgtv-cli/mgtv"
//
// The Domain also backs the standalone mgtv binary (see cli.NewApp).
package mgtv

import (
	"context"
	"regexp"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// BaseURL is used for URL construction in Locate.
const BaseURL = "https://www.mgtv.com"

func init() { kit.Register(Domain{}) }

// Domain is the Mango TV driver. It carries no state.
type Domain struct{}

// Info describes the scheme, hostnames, and identity for the kit framework.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "mgtv",
		Hosts:  []string{Host, "www.mgtv.com"},
		Identity: kit.Identity{
			Binary: "mgtv",
			Short:  "Browse Mango TV content (mgtv.com)",
			Long: `mgtv turns mgtv.com into a fast, scriptable command line.

Browse trending variety shows, dramas, and movies from Mango TV (芒果TV),
one of China's major streaming platforms - all without an account or API key.

Quick start:
  mgtv hot                        trending content from all categories
  mgtv hot --type variety -n 10   top 10 variety shows
  mgtv search 综艺                 search for variety content
  mgtv video 867784               fetch metadata for one series
  mgtv video 867784 -o json       output as JSON`,
			Site: Host,
			Repo: "https://github.com/tamnd/mgtv-cli",
		},
	}
}

// Register installs the client factory and all three MGTV operations onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "hot",
		Group:   "browse",
		Summary: "Trending/hot content from category pages",
	}, hotVideos)

	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "discover",
		Summary: "Search content by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "keyword to search for"}},
	}, searchVideos)

	kit.Handle(app, kit.OpMeta{
		Name:     "video",
		Group:    "fetch",
		Single:   true,
		Resolver: true,
		URIType:  "video",
		Summary:  "Fetch metadata for a series by clip ID",
		Args:     []kit.Arg{{Name: "id", Help: "clip ID (e.g. 867784)"}},
	}, getVideo)
}

// newClient builds a Client from the resolved kit Config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	return NewClientConfig(c), nil
}

// --- input structs ---

type hotInput struct {
	Type   string  `kit:"flag" help:"category: variety, tv, movie, all" default:"all"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg" help:"keyword to search for"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type videoInput struct {
	ID     string  `kit:"arg" help:"clip ID (e.g. 867784)"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func hotVideos(ctx context.Context, in hotInput, emit func(Video) error) error {
	videos, err := in.Client.HotVideos(ctx, in.Type, in.Limit)
	if err != nil {
		return err
	}
	for _, v := range videos {
		if err := emit(v); err != nil {
			return err
		}
	}
	return nil
}

func searchVideos(ctx context.Context, in searchInput, emit func(Video) error) error {
	videos, err := in.Client.SearchVideos(ctx, in.Query, in.Limit)
	if err != nil {
		return err
	}
	for _, v := range videos {
		if err := emit(v); err != nil {
			return err
		}
	}
	return nil
}

func getVideo(ctx context.Context, in videoInput, emit func(*Video) error) error {
	v, err := in.Client.GetVideo(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(v)
}

// --- Resolver ---

var clipIDDigits = regexp.MustCompile(`^(\d+)$`)
var clipIDFromURL = regexp.MustCompile(`/b/(\d+)`)

// Classify turns any accepted input into the canonical (uriType, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("mgtv: empty input")
	}
	// Numeric clip ID
	if clipIDDigits.MatchString(input) {
		return "video", input, nil
	}
	// URL containing /b/<id>
	if m := clipIDFromURL.FindStringSubmatch(input); m != nil {
		return "video", m[1], nil
	}
	return "", "", errs.Usage("mgtv: unrecognized reference: %q", input)
}

// Locate returns the canonical Mango TV URL for a (uriType, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "video":
		return BaseURL + "/b/" + id + ".html", nil
	default:
		return "", errs.Usage("mgtv has no resource type %q", uriType)
	}
}

// mapErr translates library errors into kit error kinds with appropriate exit codes.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if err == ErrNotFound {
		return errs.NotFound("%s", err.Error())
	}
	return err
}
