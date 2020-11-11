package fixc

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const SOH = string(1)

// length of |10=???|
const ENDLEN = 8

type FixMessage struct {
	cursor int
	raw    string
	pair   [][2]string
}

type MsgFilter struct {
	filters []string
	c       chan *FixMessage
}

type FIXClient struct {
	stop         bool
	stopCh       chan bool
	seq          int64
	q            chan string
	lock         sync.Mutex
	exceptId     int64
	excepts      map[int64]*MsgFilter
	Timeout      time.Duration
	HeartBeat    time.Duration
	Version      string
	FIXAddr      string
	sendCache    []string
	SenderCompID string
	TargetCompID string
}

func (p *FixMessage) Raw() string {
	return p.raw
}

func (p *FixMessage) String() string {
	return strings.Replace(p.raw, SOH, "|", -1)
}

func (p *FixMessage) ResetCursor() {
	p.cursor = 0
}

func (p *FixMessage) Find(k string, cuts ...string) (string, bool) {
	cursor := p.cursor
	for cursor < len(p.pair) {
		pos := cursor
		cursor++
		for _, cut := range cuts {
			if cut == p.pair[pos][0] {
				return "", false
			}
		}
		if p.pair[pos][0] == k {
			return p.pair[pos][1], true
		}
	}
	return "", false
}

func (p *FixMessage) Next(k string) (string, bool) {
	for p.cursor < len(p.pair) {
		pos := p.cursor
		p.cursor++
		if p.pair[pos][0] == k {
			return p.pair[pos][1], true
		}
	}
	return "", false
}

func (p *FixMessage) Get(k string) (string, bool) {
	for _, v := range p.pair {
		if v[0] == k {
			return v[1], true
		}
	}
	return "", false
}

func NewFixMessage(s string) *FixMessage {
	msg := &FixMessage{}
	msg.cursor = 0
	msg.raw = s
	arr := strings.Split(s, SOH)
	if len(arr) == 1 {
		arr = strings.Split(s, "|")
	}
	msg.pair = make([][2]string, len(arr))
	for i, v := range arr {
		tmp := strings.Split(v, "=")
		if len(tmp) == 2 {
			msg.pair[i] = [2]string{tmp[0], tmp[1]}
		}
	}
	return msg
}

func GUID() string {
	b := make([]byte, 48)
	io.ReadFull(rand.Reader, b)
	h := md5.New()
	h.Write([]byte(fmt.Sprintf("%d-%s-BotVS-salt", time.Now().Nanosecond(), string(b))))
	s := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[:8], s[8:12], s[12:16], s[16:20], s[20:])
}

func NewFixClient(timeout, heartBeat time.Duration, version, fixAddr, senderCompID, targetCompID string) *FIXClient {
	p := &FIXClient{}
	p.Timeout = timeout
	p.HeartBeat = heartBeat
	p.Version = version
	p.FIXAddr = fixAddr
	p.SenderCompID = senderCompID
	p.TargetCompID = targetCompID
	p.stop = true
	p.seq = 1
	p.sendCache = []string{}
	return p
}

func (p *FIXClient) Start(onConnect func(), onMessage func(*FixMessage), onError func(error)) {
	p.q = make(chan string, 1024)
	p.stop = false
	p.stopCh = make(chan bool)
	p.excepts = map[int64]*MsgFilter{}
	go func() {
		dailer := net.Dialer{Timeout: p.Timeout, KeepAlive: p.Timeout}
		retry := 0
		for !p.stop {
			p.seq = 1
			p.sendCache = []string{}
			if retry > 0 {
				time.Sleep(time.Millisecond * 500)
			}
			retry++
			conn, err := tls.DialWithDialer(&dailer, "tcp", p.FIXAddr, &tls.Config{InsecureSkipVerify: true})
			if err != nil && onError != nil {
				onError(err)
				continue
			}
			if onConnect != nil {
				go onConnect()
			}
			senderExit := make(chan bool)
			t1 := time.NewTimer(p.HeartBeat * 3)
			go func() {
				isManually := false
				t := time.NewTicker(p.HeartBeat)
				defer func() {
					t.Stop()
					conn.Close()
					if isManually {
						<-senderExit
					}
				}()
				for {
					select {
					case <-t.C:
						p.Send("35=0|49=|56=|34=|52=|")
					case <-t1.C:
						isManually = true
						if onError != nil {
							onError(errors.New("heartBeat timeout, reconnect..."))
						}
						return
					case <-senderExit:
						return
					case data := <-p.q:
						if data == "exit" {
							isManually = true
							return
						}
						p.sendCache = append(p.sendCache, data)
						if len(p.sendCache) > 100 {
							p.sendCache = p.sendCache[30:]
						}
						if _, err := conn.Write([]byte(data)); err != nil {
							if onError != nil {
								onError(err)
							}
							break
						}
					}
				}
			}()
			scanner := bufio.NewScanner(conn)
			scanner.Split(func(data []byte, isEOF bool) (advance int, token []byte, err error) {
				if isEOF && len(data) == 0 {
					return 0, nil, nil
				}
				if i := bytes.Index(data, []byte(SOH+"10=")); i >= 0 {
					// Check if we have tag 10 followed by SOH.
					if len(data)-i >= ENDLEN {
						return i + ENDLEN, data[0 : i+ENDLEN], nil
					}
				}
				// If EOF, we have a final, non-SOH terminated message.
				if isEOF {
					return len(data), data, nil
				}
				// Request more data.
				return 0, nil, nil
			})
			for scanner.Scan() {
				s := scanner.Text()
				msg := NewFixMessage(s)
				if onMessage != nil {
					onMessage(msg)
				}
				t1.Reset(p.HeartBeat * 2)
				msgType, _ := msg.Get("35")
				if msgType == "2" {
					if seqNo, ok := msg.Get("7"); ok {
						for _, cache := range p.sendCache {
							if strings.Contains(cache, SOH+"34="+seqNo+SOH) {
								// TODO
								println("resend OK", seqNo)
								break
							}
						}
						println("resend")
					}
					continue
				}
				// heartbeat
				if msgType == "0" || msgType == "1" {
					p.Send("35=0|49=|56=|34=|52=|")
					continue
				}
				// SequenceReset
				if msgType == "4" {

				}
				p.lock.Lock()
				for idx, filter := range p.excepts {
					for _, k := range filter.filters {
						if strings.Contains(s, k) {
							select {
							case filter.c <- msg:
							}
							delete(p.excepts, idx)
							break
						}
					}
				}
				p.lock.Unlock()
			}
			if err := scanner.Err(); err != nil && !p.stop {
				if onError != nil {
					onError(err)
				}
			}
			conn.Close()
			senderExit <- true
		}
		p.stopCh <- true
	}()
}

func (p *FIXClient) Send(s string) (err error) {
	t := time.Now().UTC()
	utcTime := fmt.Sprintf("%d%02d%02d-%02d:%02d:%02d.%03d", t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000000)
	input := strings.Split(s, "|")
	var body string
	// let's construct message body from input
	for _, v := range input {
		m := strings.Split(v, "=")
		if m[0] == "" ||
			m[0] == "8" ||
			m[0] == "9" ||
			m[0] == "10" {
			continue
		} else {
			tag, err := strconv.Atoi(m[0])
			if err != nil {
				return err
			}
			switch {
			case tag == 34:
				body = fmt.Sprintf("%s%d=%d|", body, tag, p.seq)
			case tag == 49:
				body = fmt.Sprintf("%s%d=%s|", body, tag, p.SenderCompID)
			case tag == 52:
				if m[1] == "" {
                    body = fmt.Sprintf("%s%d=%s|", body, tag, utcTime)
				} else {
                    body = fmt.Sprintf("%s%d=%s|", body, tag, m[1])
				}
			case tag == 56:
				body = fmt.Sprintf("%s%d=%s|", body, tag, p.TargetCompID)
			case tag == 108:
				body = fmt.Sprintf("%s%d=%d|", body, tag, p.HeartBeat/time.Second)
			default:
				body = fmt.Sprintf("%s%d=%s|", body, tag, m[1])
			}
		}
	}
	header := fmt.Sprintf("8=FIX.%s|9=%d|", p.Version, len(body))
	parsed := strings.Replace(header+body, "|", SOH, -1)
	var cksum uint
	for _, val := range []byte(parsed) {
		cksum = cksum + uint(val)
	}
	cksum = uint(math.Mod(float64(cksum), 256))
	parsed = fmt.Sprintf("%s10=%03d%s", parsed, cksum, SOH)

    printMsg := strings.Replace(parsed, SOH, "|", -1)
    fmt.Println("Send:", printMsg)

	p.seq++
	if !p.stop && p.q != nil {
		select {
		case p.q <- parsed:
		default:
		}
	}
	return nil
}

func (p *FIXClient) Expect(filter ...string) (msg *FixMessage, err error) {
	c := make(chan *FixMessage)
	p.lock.Lock()
	p.exceptId++
	exceptId := p.exceptId
	p.excepts[exceptId] = &MsgFilter{filter, c}
	p.lock.Unlock()
	select {
	case msg = <-c:
	case <-time.After(p.Timeout):
		p.lock.Lock()
		delete(p.excepts, exceptId)
		p.lock.Unlock()
		err = errors.New("timeout")
	}
	close(c)
	return
}

func (p *FIXClient) Stop() {
	if !p.stop {
		p.stop = true
		select {
		case p.q <- "exit":
		default:
		}
		<-p.stopCh
		close(p.stopCh)
	}
	if p.q != nil {
		close(p.q)
		p.q = nil
	}
}
