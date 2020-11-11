package fixc

import (
	"bytes"
	"fmt"
	"strconv"
)

const (
	EncryptMethod_NONE_OTHER  = 0
	EncryptMethod_PKCS        = 1
	EncryptMethod_DES         = 2
	EncryptMethod_PKCS_DES    = 3
	EncryptMethod_PGP_DES     = 4
	EncryptMethod_PGP_DES_MD5 = 5
	EncryptMethod_PEM_DES_MD5 = 6
)

const (
	FID_EncryptMethod = 98
	FID_HeartBtInt    = 108
)

type IMsg interface {
	Pack() string
}

type MsgBase struct {
	fields map[int]string
}

func (p *MsgBase) AddGroup(fid int, msg IMsg) {
	if p.fields == nil {
		p.fields = map[int]string{}
	}
	p.fields[fid] = msg.Pack()
}

func (p *MsgBase) AddField(fid int, s interface{}) {
	if p.fields == nil {
		p.fields = map[int]string{}
	}
	var ret string
	switch v := s.(type) {
	case string:
		ret = v
	case int64:
		ret = strconv.FormatInt(v, 10)
	case int32:
		ret = strconv.FormatInt(int64(v), 10)
	case int:
		ret = strconv.FormatInt(int64(v), 10)
	case float64:
		ret = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		ret = strconv.FormatBool(v)
	default:
		ret = fmt.Sprintf("%v", s)
	}
	p.fields[fid] = ret
}

func (p *MsgBase) Pack() string {
	var b bytes.Buffer
	isFirst := true
	for fid, v := range p.fields {
		if isFirst {
			isFirst = false
			b.WriteString(fmt.Sprintf("%d=%s", fid, v))
		} else {
			b.WriteString(fmt.Sprintf("|%d=%s", fid, v))
		}
	}
	return b.String()
}

type MsgLogon struct {
	MsgBase
}

func (p *MsgLogon) SetEncryptMethod(v int) {
	p.AddField(FID_EncryptMethod, v)
}
func (p *MsgLogon) SetHeartBtInt(v int) {
	p.AddField(FID_HeartBtInt, v)
}
