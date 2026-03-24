package domain

type Mode string

const (
	ModeStd    Mode = "std"
	ModeLax    Mode = "lax"
	ModeYolo   Mode = "yolo"
	ModeStrict Mode = "strict"
)

type WorkspaceMode string

const (
	WorkspaceModeMount WorkspaceMode = "mount"
	WorkspaceModeCopy  WorkspaceMode = "copy"
)

type Profile string

type GearName string

type WorkspaceRef struct {
	Input      string
	Dir        string
	RepoRoot   string
	ConfigPath string
	Slug       string
	Hash       string
}

type DuneConfig struct {
	Profile       Profile
	Mode          Mode
	WorkspaceMode WorkspaceMode
	Gear          []GearName

	PythonVersion string
	UVVersion     string
	GoVersion     string
	RustVersion   string

	UnknownKeys []string
}

type ResolvedConfig struct {
	Workspace       WorkspaceRef
	Config          DuneConfig
	Warnings        []string
	ProfileExplicit bool
	ModeExplicit    bool
}

type ContainerIdentity struct {
	Name string
}

type ContainerState struct {
	Exists        bool
	Running       bool
	Mode          Mode
	WorkspaceMode WorkspaceMode
}

type GearSpec struct {
	Name           GearName
	Script         string
	Description    string
	EnabledModes   map[Mode]bool
	RunAs          string
	HelperCommands []string
}
