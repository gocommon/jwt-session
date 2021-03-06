package jwt

import (
	"fmt"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
)

// Author Author
type Author struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Avator string `json:"avator"`
}

// SessionClaims SessionClaims
type SessionClaims struct {
	Author  Author                 `json:"author"`
	CAAType CAAType                `json:"caa_type"`
	CAA     SessionCAA             `json:"caa"`    // 记录有效用户序号
	Verify  map[string]interface{} `json:"verify"` // 验证数据，默认存ip,host
	Data    map[string]interface{} `json:"data"`   // 用户自定义数据
	jwt.StandardClaims
}

// NewSessionClaims NewSessionClaims
func NewSessionClaims() *SessionClaims {
	return &SessionClaims{
		Verify: make(map[string]interface{}),
		Data:   make(map[string]interface{}),
	}
}

// SetAuthor  自定义数据
func (p *SessionClaims) SetAuthor(author Author) *SessionClaims {
	p.Author = author
	return p
}

// GetData GetData
func (p *SessionClaims) GetData(key string) interface{} {
	return p.Data[key]
}

// SetData SetData
func (p *SessionClaims) SetData(key string, value interface{}) *SessionClaims {
	p.Data[key] = value
	return p
}

// VerifyKey VerifyKey
func (p *SessionClaims) VerifyKey(key string, value interface{}) bool {
	return p.Verify[key] == value
}

// SetVerify SetVerify
func (p *SessionClaims) SetVerify(key string, value interface{}) *SessionClaims {
	p.Verify[key] = value
	return p
}

// SetCAA SetCAA
func (p *SessionClaims) SetCAA(caa SessionCAA) *SessionClaims {
	p.CAA = caa
	return p
}

// Session Session
type Session struct {
	opts  Options
	store Store
	*jwt.Token
}

// Author Author
func (p *Session) Author() Author {
	return p.GetCliams().Author
}

// SetAuthor  自定义数据
func (p *Session) SetAuthor(author Author) *Session {
	p.GetCliams().SetAuthor(author)
	return p
}

// Data Data
func (p *Session) Data() map[string]interface{} {
	return p.GetCliams().Data
}

// SetData SetData
func (p *Session) SetData(key string, value interface{}) *Session {
	p.GetCliams().SetData(key, value)
	return p
}

// GetCliams GetCliams
func (p *Session) GetCliams() *SessionClaims {
	return p.Claims.(*SessionClaims)
}

// VerifyIP VerifyIP
func (p *Session) VerifyIP(r *http.Request) bool {
	return p.GetCliams().VerifyKey("ip", RealIP(r))
}

// Valid Valid
func (p *Session) Valid() (bool, error) {
	uid := p.Author().ID
	// uid can not be zero
	if uid == 0 {
		return false, jwt.NewValidationError("uid == 0", jwt.ValidationErrorMalformed)
	}

	if !p.Token.Valid {
		return false, jwt.NewValidationError("Token.Valid", jwt.ValidationErrorMalformed)
	}

	// caa类型
	if p.GetCliams().CAAType != p.opts.CAAType {
		return false, jwt.NewValidationError("CAAType faild", jwt.ValidationErrorMalformed)
	}

	if p.opts.UseCounter() {
		c, err := p.store.GetCounter(uid)
		if err != nil {
			return false, err
		}

		cp := Counter(c)

		valid := cp.IsValid(p.GetCliams().CAA, int64(p.opts.MaxActive))
		if !valid {
			return false, jwt.NewValidationError(fmt.Sprintf("caa counter faild %d+%d=%d", p.GetCliams().CAA, int64(p.opts.MaxActive), cp), jwt.ValidationErrorMalformed)
		}
		// fmt.Printf("caa timecounterout ok %d+%d=%d\n", p.GetCliams().CAA, int64(p.opts.MaxActive), cp)
		return valid, nil
	}

	c, err := p.store.GetTimeout(uid)
	if err != nil {
		return false, err
	}

	cp := Timeout(c)

	valid := cp.IsValid(p.GetCliams().CAA, int64(p.opts.MaxAge)*86400)
	if !valid {
		return false, jwt.NewValidationError(fmt.Sprintf("caa timeout faild %d+%d=%d", p.GetCliams().CAA, int64(p.opts.MaxAge)*86400, cp), jwt.ValidationErrorMalformed)
	}

	// fmt.Printf("caa timeout ok %d+%d=%d\n", p.GetCliams().CAA, int64(p.opts.MaxAge)*86400, cp)
	return valid, nil
}

// SignedString SignedString 生成SignedString
func (p *Session) SignedString() (string, error) {
	var (
		token string
		err   error
	)

	if p.opts.UseCounter() {
		c, err := p.store.GetCounter(p.Author().ID)
		if err != nil {
			return "", err
		}

		cp := Counter(c)
		// @note 并发登陆的时候，可能会出现多个sessionCAA一样的情况，机率很小
		p.GetCliams().SetCAA(cp.Issue())

		defer func() {
			// counter 每次登陆+1， 要更新store
			err = p.store.SetCounter(p.Author().ID, int64(cp))
		}()
	} else {
		c, err := p.store.GetTimeout(p.Author().ID)
		if err != nil {
			return "", err
		}

		cp := Timeout(c)

		// timout 首次分配时更新就行
		if !cp.HasIssued() {
			defer func() {
				err = p.store.SetTimeout(p.Author().ID, int64(cp))
			}()
		}

		p.GetCliams().SetCAA(cp.Issue())

	}

	var startTime time.Time

	if p.opts.MinNight.True() {
		startTime = MinNight()
	} else {
		startTime = time.Now()
	}

	p.GetCliams().ExpiresAt = startTime.AddDate(0, 0, p.opts.MaxAge).Unix()
	p.GetCliams().IssuedAt = time.Now().Unix()

	token, err = p.Token.SignedString(p.opts.GetSignKey(p.opts.JwtSigningMethod))
	if err != nil {
		return "", err
	}

	return token, err
}

// Flush Flush
func (p *Session) Flush() error {
	var err error
	uid := p.Author().ID
	// uid can not be zero
	if uid == 0 {
		return nil
	}

	if p.opts.UseCounter() {
		c, err := p.store.GetCounter(p.Author().ID)
		if err != nil {
			return err
		}

		cp := Counter(c)

		// timout 没有分配说明没有登陆过
		if !cp.HasIssued() {
			return nil
		}

		cp.Revoke(int64(p.opts.MaxActive))

		// counter， 要更新store
		err = p.store.SetCounter(p.Author().ID, int64(cp))

	} else {
		c, err := p.store.GetTimeout(p.Author().ID)
		if err != nil {
			return err
		}

		cp := Timeout(c)

		// timout 没有分配说明没有登陆过
		if !cp.HasIssued() {
			return err
		}

		cp.Revoke(Now().Unix())

		// counter， 要更新store
		err = p.store.SetTimeout(p.Author().ID, int64(cp))

	}

	return err
}
