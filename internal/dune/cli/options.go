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
	CommandDestroy     Command = "destroy"
	CommandRebuild     Command = "rebuild"
	CommandLogs        Command = "logs"
	CommandPorts       Command = "ports"
	CommandDoctor      Command = "doctor"
	CommandVersion     Command = "version"
	CommandHelp        Command = "help"
	CommandUpdate      Command = "update"
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
	Force           bool
	Verbose         bool
	JSON            bool
	PortsPublish    []string
	PortsUnpublish  []string
}

func Parse(argv []string) (Options, error) {
	if len(argv) > 0 {
		switch argv[0] {
		case "-v", "--version":
			return parseVersion(argv[1:])
		case "-h", "--help":
			return parseHelp(argv[1:])
		case "-u", "--update":
			return parseUpdate(argv[1:])
		}
	}

	if len(argv) == 0 {
		return parseContainerCommand(CommandUp, "dune", nil)
	}

	switch argv[0] {
	case "up":
		return parseContainerCommand(CommandUp, "dune up", argv[1:])
	case "down":
		return parseContainerCommand(CommandDown, "dune down", argv[1:])
	case "destroy":
		return parseDestroy(argv[1:])
	case "rebuild":
		return parseContainerCommand(CommandRebuild, "dune rebuild", argv[1:])
	case "logs":
		return parseLogs(argv[1:])
	case "ports":
		return parsePorts(argv[1:])
	case "doctor":
		return parseDoctor(argv[1:])
	case "version":
		return parseVersion(argv[1:])
	case "profile":
		return parseProfile(argv[1:])
	default:
		return parseContainerCommand(CommandUp, "dune", argv)
	}
}

func parseDoctor(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: CommandDoctor}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
	fs.BoolVar(&opts.JSON, "json", false, "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("usage: dune doctor [--json] [--verbose] [-d directory] [-p profile]")
	}
	if len(args) == 1 {
		if opts.WorkspaceInput != "" {
			return Options{}, errors.New("usage: dune doctor [--json] [--verbose] [-d directory] [-p profile]")
		}
		opts.WorkspaceInput = args[0]
	}
	opts.ProfileExplicit = opts.Profile != ""
	return opts, nil
}

func parseDestroy(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune destroy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: CommandDestroy}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	fs.BoolVar(&opts.Force, "force", false, "")
	fs.BoolVar(&opts.Force, "f", false, "")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("usage: dune destroy [-f|--force] [-d directory] [-p profile]")
	}
	if len(args) == 1 {
		if opts.WorkspaceInput != "" {
			return Options{}, errors.New("usage: dune destroy [-f|--force] [-d directory] [-p profile]")
		}
		opts.WorkspaceInput = args[0]
	}
	opts.ProfileExplicit = opts.Profile != ""
	return opts, nil
}

func parseContainerCommand(command Command, name string, argv []string) (Options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: command}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
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
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
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

func parsePorts(argv []string) (Options, error) {
	fs := flag.NewFlagSet("dune ports", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := Options{Command: CommandPorts}
	fs.StringVar(&opts.WorkspaceInput, "directory", "", "")
	fs.StringVar(&opts.WorkspaceInput, "d", "", "")
	fs.StringVar(&opts.Profile, "profile", "", "")
	fs.StringVar(&opts.Profile, "p", "", "")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
	fs.Var(&stringSliceFlag{slice: &opts.PortsPublish}, "publish", "")
	fs.Var(&stringSliceFlag{slice: &opts.PortsUnpublish}, "unpublish", "")
	if err := fs.Parse(argv); err != nil {
		return Options{}, err
	}

	args := fs.Args()
	if len(args) > 1 {
		return Options{}, errors.New("usage: dune ports [--publish <spec>] [--unpublish <spec>] [-d directory] [-p profile]")
	}
	if len(args) == 1 {
		if opts.WorkspaceInput != "" {
			return Options{}, errors.New("usage: dune ports [--publish <spec>] [--unpublish <spec>] [-d directory] [-p profile]")
		}
		opts.WorkspaceInput = args[0]
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

func parseHelp(argv []string) (Options, error) {
	if len(argv) != 0 {
		return Options{}, errors.New("usage: dune --help")
	}
	return Options{Command: CommandHelp}, nil
}

func parseUpdate(argv []string) (Options, error) {
	if len(argv) != 0 {
		return Options{}, errors.New("usage: dune --update")
	}
	return Options{Command: CommandUpdate}, nil
}

// stringSliceFlag accumulates repeatable string flags (e.g. --publish a
// --publish b) into the backing slice. It supports the per-spec repeatable
// surface that sbx ports exposes via --publish/--unpublish.
type stringSliceFlag struct {
	slice *[]string
}

func (f stringSliceFlag) String() string {
	if f.slice == nil {
		return ""
	}
	return strings.Join(*f.slice, ",")
}

func (f stringSliceFlag) Set(value string) error {
	*f.slice = append(*f.slice, value)
	return nil
}
