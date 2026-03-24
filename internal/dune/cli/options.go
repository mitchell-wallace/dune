package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"claudebox/internal/dune/config"
	"claudebox/internal/dune/domain"
)

type Command string

const (
	CommandRun     Command = "run"
	CommandConfig  Command = "config"
	CommandRebuild Command = "rebuild"
)

type Options struct {
	Command         Command
	WorkspaceInput  string
	Profile         domain.Profile
	Mode            domain.Mode
	ProfileExplicit bool
	ModeExplicit    bool
}

func Parse(argv []string) (Options, error) {
	if len(argv) > 0 {
		switch argv[0] {
		case "config":
			return parseConfig(argv[1:])
		case "rebuild":
			return parseRebuild(argv[1:])
		}
	}
	return parseRun(argv)
}

func parseConfig(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts Options
	opts.Command = CommandConfig
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("unexpected arguments for dune config")
	}
	if len(args) == 1 {
		opts.WorkspaceInput = args[0]
	}
	return opts, nil
}

func parseRebuild(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune rebuild", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts Options
	opts.Command = CommandRebuild
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("unexpected arguments for dune rebuild")
	}
	if len(args) == 1 {
		opts.WorkspaceInput = args[0]
	}
	return opts, nil
}

func parseRun(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		workspaceInput string
		profileRaw     string
		modeRaw        string
	)

	fs.StringVar(&workspaceInput, "directory", "", "")
	fs.StringVar(&workspaceInput, "d", "", "")
	fs.StringVar(&profileRaw, "profile", "", "")
	fs.StringVar(&profileRaw, "p", "", "")
	fs.StringVar(&modeRaw, "mode", "", "")
	fs.StringVar(&modeRaw, "m", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	opts := Options{Command: CommandRun, WorkspaceInput: workspaceInput}
	if profileRaw != "" {
		profile, ok := config.NormalizeProfile(profileRaw)
		if !ok {
			return Options{}, fmt.Errorf("invalid profile %q", profileRaw)
		}
		opts.Profile = profile
		opts.ProfileExplicit = true
	}
	if modeRaw != "" {
		mode, ok := config.CanonicalizeMode(modeRaw)
		if !ok {
			return Options{}, fmt.Errorf("invalid mode %q", modeRaw)
		}
		opts.Mode = mode
		opts.ModeExplicit = true
	}

	for _, token := range fs.Args() {
		if !opts.ProfileExplicit && opts.Profile == "" && config.IsProfileToken(token) {
			profile, _ := config.NormalizeProfile(token)
			opts.Profile = profile
			opts.ProfileExplicit = true
			continue
		}
		if mode, ok := config.CanonicalizeMode(token); ok {
			opts.Mode = mode
			opts.ModeExplicit = true
			continue
		}
		if opts.WorkspaceInput == "" {
			opts.WorkspaceInput = token
			continue
		}
		return Options{}, fmt.Errorf("unexpected argument: %s", token)
	}

	return opts, nil
}
