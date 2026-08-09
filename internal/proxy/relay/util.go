package relay

import "time"

// NowUnix 返回当前 Unix 时间戳
func NowUnix() int64 {
	return time.Now().Unix()
}

// nowUnix 返回当前 Unix 时间戳
func nowUnix() int64 {
	return time.Now().Unix()
}
