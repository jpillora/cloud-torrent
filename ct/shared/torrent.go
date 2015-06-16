package shared

//cloud torrent specific torrent structs

type Torrent struct {
	Name     string
	Loaded   bool
	Progress int64
	Size     int64
	InfoHash string //hash of torrent
	Files    []*File
}

type File struct {
	Path      string
	Size      int64
	Chunks    int
	Completed int
}
