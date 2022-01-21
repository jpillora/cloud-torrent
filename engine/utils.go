package engine

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/c2h5oh/datasize"
	"golang.org/x/time/rate"
)

func rateLimiter(rstr string) (*rate.Limiter, error) {
	var rateSize int
	rstr = strings.ToLower(strings.TrimSpace(rstr))
	switch rstr {
	case "low":
		// ~50k/s
		rateSize = 50000
	case "medium":
		// ~500k/s
		rateSize = 500000
	case "high":
		// ~1500k/s
		rateSize = 1500000
	case "unlimited", "0", "":
		// unlimited
		return rate.NewLimiter(rate.Inf, 0), nil
	default:
		var v datasize.ByteSize
		err := v.UnmarshalText([]byte(rstr))
		if err != nil {
			return nil, err
		}
		if v > 2147483647 {
			// max of int, unlimited
			return nil, errors.New("excceed int val")
		}

		rateSize = int(v)
	}
	return rate.NewLimiter(rate.Limit(rateSize), rateSize*3), nil
}

func cmdScanLine(p io.ReadCloser, wg *sync.WaitGroup, logprefix string) {
	sc := bufio.NewScanner(p)
	for sc.Scan() {
		oline := strings.TrimSpace(sc.Text())
		if len(oline) > 0 {
			log.Println(logprefix, oline)
		}
	}

	wg.Done()
}

func mkdir(dirpath string) {
	if st, err := os.Stat(dirpath); errors.Is(err, os.ErrNotExist) {
		common.HandleError(os.MkdirAll(dirpath, os.ModePerm))
	} else if !st.IsDir() {
		log.Panic("[FATAL] path exists but is not a directory, please remove it:", dirpath)
	}
}

func fetchTxtList(url string) ([]string, error) {
	var txtlines []string

	log.Println("fetchTxtList: fetching", url)
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		txtlines = append(txtlines, line)
	}

	log.Println("fetchTxtList: got lines", len(txtlines))
	return txtlines, nil
}
