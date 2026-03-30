package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type Command string

const (
	CommandUp          Command = "up"
	CommandDown        Command = "down"
	CommandRebuild     Command = "rebuild"
	CommandLogs        Command = "logs"
	CommandVersion     Command = "version"
	CommandProfileSet  Command = "profile-set"
	CommandProfileList Command = "profile-list"
)

type Options struct {
	Command         Command
	WorkspaceInput  string
	Profile         string
	ProfileExplicit bool
	LogService      string
	SetProfileName  string
}

func Parse(argv []string) (Options, error) {
	if len(argv) == 0 {
		return parseContainerCommand(CommandUp, "dune", nil)
	}

	switch argv[0] {
	case "up":
		return parseContainerCommand(CommandUp, "dune up", argv[1:])
	case "down":
		return parseContainerCommand(CommandDown, "dune down", argv[1:])
	case "rebuild":
		return parseContainerCommand(CommandRebuild, "dune rebuild", argv[1:])
	case "logs":
		return parseLogs(argv[1:])
	case "version":
		return parseVersion(argv[1:])
	case "profile":
		return parseProfile(argv[1:])
	default:
		return parseContainerCommand(CommandUp, "dune", argv)
	}
}

func parseContainerCommand(command Command, name string, argv []string) (Options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: command}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if len(args) > 1 {
		return Options{}, fmt.Errorf("unexpected arguments for %s", name)
	}
	if len(args) == 1 {
		if opts.WorkspaceInput != "" {
			return Options{}, fmt.Errorf("unexpected arguments for %s", name)
		}
		opts.WorkspaceInput = args[0]
	}
	opts.ProfileExplicit = opts.Profile != ""
	return opts, nil
}

func parseLogs(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: CommandLogs}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("usage: dune logs [service] [-d directory] [-p profile]")
	}
	if len(args) == 1 {
		opts.LogService = args[0]
	}
	opts.ProfileExplicit = opts.Profile != ""
	return opts, nil
}

func parseProfile(argv []string) (Options, error) {
	if len(argv) == 0 {
		return Options{}, errors.New("usage: dune profile <set|list> [args]")
	}

	switch argv[0] {
	case "set":
		return parseProfileSet(argv[1:])
	case "list":
		return parseProfileList(argv[1:])
	default:
		return Options{}, errors.New("usage: dune profile <set|list> [args]")
	}
}

func parseProfileSet(argv []string) (Options, error) {
	if len(argv) == 0 {
		return Options{}, errors.New("usage: dune profile set <name> [-d directory]")
	}

	opts := Options{Command: CommandProfileSet}
	if !strings.HasPrefix(argv[0], "-") {
		opts.SetProfileName = argv[0]
		argv = argv[1:]
	}

	fs := flag.NewFlagSet("dune profile set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if opts.SetProfileName == "" {
		if len(args) < 1 || len(args) > 2 {
			return Options{}, errors.New("usage: dune profile set <name> [-d directory]")
		}
		opts.SetProfileName = args[0]
		if len(args) == 2 {
			opts.WorkspaceInput = args[1]
		}
	} else if len(args) > 1 {
		return Options{}, errors.New("usage: dune profile set <name> [-d directory]")
	} else if len(args) == 1 {
		opts.WorkspaceInput = args[0]
	}
	return opts, nil
}

func parseProfileList(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune profile list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: CommandProfileList}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}
	if len(fs.Args()) != 0 {
		return Options{}, errors.New("usage: dune profile list [-d directory]")
	}
	return opts, nil
}

func parseVersion(argv []string) (Options, error) {
	if len(argv) != 0 {
		return Options{}, errors.New("usage: dune version")
	}
	return Options{Command: CommandVersion}, nil
}
