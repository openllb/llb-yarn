package yarn

import (
	"context"
	"path/filepath"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"
)

var (
	CopyOptions = &llb.CopyInfo{
		FollowSymlinks:      true,
		CopyDirContentsOnly: true,
		AttemptUnpack:       false,
		CreateDestPath:      true,
		AllowWildcard:       true,
		AllowEmptyWildcard:  true,
	}
)

func Install(ctx context.Context, c client.Client) (*client.Result, error) {
	st, err := NewState(ctx, c)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate llb")
	}

	def, err := st.Marshal()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal local source")
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve dockerfile")
	}

	return res, nil
}

func NewState(ctx context.Context, c client.Client) (*llb.State, error) {
	inputs := []string{"package.json", "package-lock.json", ".npmrc", ".yarnrc"}
	src := llb.Local("context",
		llb.FollowPaths(inputs),
	)

	yarn := llb.Image("docker.io/library/node:alpine")

	installPath := "/opt/output"
	yarn = yarn.File(
		llb.Copy(src, "/", installPath, CopyOptions),
	)

	yarn = yarn.Dir(installPath).Run(
		llb.Shlexf("yarn install --non-interactive --pure-lockfile"),
		llb.AddMount(
			"/usr/local/share/.cache/yarn/v6",
			llb.Scratch(),
			llb.AsPersistentCacheDir("openllb/llb-yarn", llb.CacheMountShared),
		),
		ControlCache(c),
	).Root()

	output := llb.Scratch().File(
		llb.Copy(yarn, filepath.Join(installPath, "node_modules"), "/", CopyOptions),
	)

	return &output, nil
}

// ControlCache will force the cache to be ignored id the `--no-cache`
// option is specified via buildctl
func ControlCache(client client.Client) ConstraintsOptFunc {
	return ConstraintsOptFunc(func(c *llb.Constraints) {
		if _, ok := client.BuildOpts().Opts["no-cache"]; ok {
			c.Metadata.IgnoreCache = true
		}
	})
}

// ConstraintsOptFunc is a copy of llb.constrainsOptFunc but that is private
// so we reimplement here.  There interface should probably be public
// public
type ConstraintsOptFunc func(c *llb.Constraints)

func (fn ConstraintsOptFunc) SetRunOption(ei *llb.ExecInfo) {
	fn(&ei.Constraints)
}

func (fn ConstraintsOptFunc) SetConstraintsOption(c *llb.Constraints) {
	fn(c)
}

func (fn ConstraintsOptFunc) SetGitOption(gi *llb.GitInfo) {
	fn(&gi.Constraints)
}

func (fn ConstraintsOptFunc) SetHTTPOption(hi *llb.HTTPInfo) {
	fn(&hi.Constraints)
}

func (fn ConstraintsOptFunc) SetImageOption(ii *llb.ImageInfo) {
	fn(&ii.Constraints)
}

func (fn ConstraintsOptFunc) SetLocalOption(li *llb.LocalInfo) {
	fn(&li.Constraints)
}
