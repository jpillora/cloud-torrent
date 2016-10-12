package realtime

type jsonBytes []byte

func (j jsonBytes) MarshalJSON() ([]byte, error) {
	return []byte(j), nil
}
