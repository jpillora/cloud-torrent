package ct

//cloud torrent specific torrent structs

type Torrent struct {
	Name  string
	Size  int64
	Speed struct {
		Up   int64
		Down int64
	}
	InfoHash string
	Progress int
	Files    []*File
}

type File struct {
	Name     string
	Size     int64
	Progress int
	Hash     string
}
