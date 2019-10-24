package yarn

import (
	"context"
	"encoding/json"
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
	inputs := []string{"package.json", "yarn.lock", ".npmrc", ".yarnrc"}
	src := llb.Local("context",
		llb.FollowPaths(inputs),
		llb.WithCustomNamef("load %q", inputs),
	)

	ref := c.BuildOpts().Opts["yarn"]
	if ref == "" {
		ref = "docker.io/library/node:alpine"
	}

	yarn := llb.Image(ref)

	installPath := "/opt/output"
	yarn = yarn.File(
		llb.Copy(src, "/", installPath, CopyOptions),
	)

	patterns, err := WorkspacePatterns(ctx, c, src)
	if err != nil {
		return nil, err
	}

	if len(patterns) > 0 {
		workspaces := llb.Local("context",
			llb.IncludePatterns(patterns),
			llb.WithCustomNamef("load workspaces %q", patterns),
		)

		yarn = yarn.File(
			llb.Copy(workspaces, "/", installPath, CopyOptions),
		)
	}

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

func WorkspacePatterns(ctx context.Context, c client.Client, src llb.State) ([]string, error) {
	caps := c.BuildOpts().LLBCaps
	marshalOpts := []llb.ConstraintsOpt{llb.WithCaps(caps)}

	def, err := src.Marshal(marshalOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal local source")
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve dockerfile")
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	content, err := ref.ReadFile(ctx, client.ReadRequest{
		Filename: "package.json",
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read package.json")
	}

	var pkg map[string]interface{}
	err = json.Unmarshal(content, &pkg)
	if err != nil {
		return nil, err
	}

	var patterns []string

	workspacesRaw, ok := pkg["workspaces"]
	if ok {
		workspaces, ok := workspacesRaw.([]interface{})
		if ok {
			for _, workspaceRaw := range workspaces {
				workspace, ok := workspaceRaw.(string)
				if !ok {
					continue
				}

				patterns = append(patterns, filepath.Join(workspace, "package.json"))
			}
		}
	}

	return patterns, nil
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
