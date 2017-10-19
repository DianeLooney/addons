package addons

type File struct {
	Path    string
	Hash    []byte
	Content []byte
}

type Manifest struct {
	Files []*File
}

type Package struct {
	PackageID   string
	Title       string
	Interface   int
	Version     string
	Directories []string
	Manifest    *Manifest
}

type Addon struct {
	Name     string
	Packages []*Package
}
