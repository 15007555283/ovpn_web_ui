package app

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Online struct {
	Username  string    `json:"username"`
	VPNIP     string    `json:"vpn_ip"`
	Remote    string    `json:"remote"`
	Connected time.Time `json:"connected_at"`
	Received  uint64    `json:"bytes_received"`
	Sent      uint64    `json:"bytes_sent"`
}
type OnlineStatus struct {
	Available bool      `json:"available"`
	Message   string    `json:"message"`
	Updated   time.Time `json:"updated_at"`
	Clients   []Online  `json:"clients"`
}

// status-version 3 使用制表符；依据 HEADER 定位字段，兼容新增列。
func parseStatus(raw string, now time.Time) (OnlineStatus, error) {
	out := OnlineStatus{Clients: []Online{}}
	header := map[string]int{}
	ended := false
	scan := bufio.NewScanner(strings.NewReader(raw))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	for scan.Scan() {
		line := strings.Split(scan.Text(), "\t")
		if ended && strings.TrimSpace(scan.Text()) != "" {
			return out, errors.New("状态文件尾部不完整")
		}
		switch line[0] {
		case "TIME":
			if len(line) < 3 {
				return out, errors.New("状态时间缺失")
			}
			n, e := strconv.ParseInt(line[2], 10, 64)
			if e != nil {
				return out, e
			}
			out.Updated = time.Unix(n, 0)
		case "HEADER":
			if len(line) > 2 && line[1] == "CLIENT_LIST" {
				for i, k := range line[2:] {
					header[k] = i + 1
				}
			}
		case "CLIENT_LIST":
			get := func(k string) (string, error) {
				i, ok := header[k]
				if !ok || i >= len(line) {
					return "", errors.New("状态文件字段缺失")
				}
				return line[i], nil
			}
			var c Online
			var e error
			if c.Username, e = get("Common Name"); e != nil {
				return out, e
			}
			if c.Remote, e = get("Real Address"); e != nil {
				return out, e
			}
			if c.VPNIP, e = get("Virtual Address"); e != nil {
				return out, e
			}
			v, e := get("Bytes Received")
			if e != nil {
				return out, e
			}
			c.Received, e = strconv.ParseUint(v, 10, 64)
			if e != nil {
				return out, e
			}
			v, e = get("Bytes Sent")
			if e != nil {
				return out, e
			}
			c.Sent, e = strconv.ParseUint(v, 10, 64)
			if e != nil {
				return out, e
			}
			v, e = get("Connected Since (time_t)")
			if e != nil {
				return out, e
			}
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				return out, e
			}
			c.Connected = time.Unix(n, 0)
			out.Clients = append(out.Clients, c)
		case "END":
			ended = true
		}
	}
	if scan.Err() != nil || !ended || len(header) == 0 || out.Updated.IsZero() || now.Sub(out.Updated) > 2*time.Minute || out.Updated.After(now.Add(time.Minute)) {
		return out, errors.New("状态暂不可用：文件缺失、截断或已过期")
	}
	out.Available = true
	return out, nil
}
