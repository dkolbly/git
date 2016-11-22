package git

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Stamp struct {
	UserName  string
	Email     string
	Timestamp time.Time
}

func (s *Stamp) String() string {
	return fmt.Sprintf("%s <%s> %s",
		s.UserName,
		s.Email,
		s.Timestamp.Format("Mon Jan 2 15:04:05 2006 MST"), // matches git output
	)
}

type Commit struct {
	name      Ptr
	raw       []byte
	repo      *Git
	Tree      Ptr
	Parent    Ptr
	Author    *Stamp
	Committer *Stamp
	Message   string
}

func (c *Commit) Type() ObjType {
	return ObjCommit
}

func (c *Commit) Payload() ([]byte, error) {
	return c.raw, nil
}

func (c *Commit) Load() (GitObject, error) {
	return c, nil
}

func (c *Commit) Name() *Ptr {
	return &c.name
}

func (g *Git) loadCommit(name *Ptr, buf []byte) (*Commit, error) {
	c := &Commit{
		name: *name,
		repo: g,
		raw:  buf,
	}
	r := bytes.NewBuffer(buf)

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		k := bytes.IndexByte(line, ' ')
		//fmt.Printf("%q %d\n", line, k)
		if k < 0 {
			break
		}
		rest := line[k+1 : len(line)-1]
		//fmt.Printf("[%s] %q\n", string(line[:k]), rest)
		switch string(line[:k]) {
		case "tree":
			ref, err := g.ExpandRef(string(rest))
			if err != nil {
				panic(err)
			}
			c.Tree = *ref
		case "parent":
			ref, err := g.ExpandRef(string(rest))
			if err != nil {
				panic(err)
			}
			c.Parent = *ref
		case "author":
			s, err := parseStamp(rest)
			if err != nil {
				panic(err)
			}
			c.Author = s
			//fmt.Printf("author %s\n", s)
		case "committer":
			s, err := parseStamp(rest)
			if err != nil {
				panic(err)
			}
			c.Committer = s
			//fmt.Printf("commiter %s\n", s)
		}
	}
	c.Message = r.String()
	//fmt.Printf("message %q\n", c.Message)
	//fmt.Printf("***\n%#v\n", c)
	return c, nil
}

var stampPat = regexp.MustCompile(`(.*)\s+<([^>]+)> (\d+) ([+-]?[0-9]+)`)

func parseStamp(buf []byte) (*Stamp, error) {
	sub := stampPat.FindStringSubmatch(string(buf))
	//fmt.Printf("%q --> %#v\n", buf, sub)

	timeSec, err := strconv.ParseInt(sub[3], 10, 63)
	if err != nil {
		return nil, err
	}

	loc, err := parseOffset(sub[4])
	if err != nil {
		return nil, err
	}

	return &Stamp{
		UserName:  sub[1],
		Email:     sub[2],
		Timestamp: time.Unix(timeSec, 0).In(loc),
	}, nil
}

var tzCacheLock sync.RWMutex
var tzCache = map[string]*time.Location{
	"-0500": time.FixedZone("-0500", -5*3600),
	"-0600": time.FixedZone("-0600", -6*3600),
}

func parseOffset(tz string) (*time.Location, error) {
	tzCacheLock.RLock()
	loc, ok := tzCache[tz]
	tzCacheLock.RUnlock()

	if ok {
		return loc, nil
	}

	hours, err := strconv.ParseInt(tz[:len(tz)-2], 10, 10)
	if err != nil {
		return nil, err
	}
	min, err := strconv.ParseInt(tz[len(tz)-2:], 10, 10)
	if err != nil {
		return nil, err
	}
	if hours < -24 || hours > 24 {
		return nil, ErrInvalidTimeZone
	}
	if min < 0 || min >= 60 {
		return nil, ErrInvalidTimeZone
	}
	if hours < 0 {
		min = -min
	}

	offsetSec := int(hours)*3600 + int(min)*60

	loc = time.FixedZone(tz, offsetSec)
	tzCacheLock.Lock()
	tzCache[tz] = loc
	tzCacheLock.Unlock()
	return loc, nil
}

var ErrInvalidTimeZone = errors.New("invalid time zone")
