// Package auth handles Proton API authentication, session persistence, and
// secure key-password storage via the OS keyring (libsecret / Secret Service).
//
// Login flow:
//  1. SRP exchange → access/refresh tokens
//  2. 2FA if required
//  3. Derive salted key password from login password + Proton key salt
//  4. Store key password in OS keyring; store tokens on disk
//
// Mount flow (no password prompt):
//  1. Load tokens from disk → create API client
//  2. Read salted key password from OS keyring
//  3. Unlock address key rings with proton.Unlock
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	proton "github.com/ProtonMail/go-proton-api"
	pgpcrypto "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/zalando/go-keyring"
)

const (
	apiBase        = "https://mail.proton.me/api"
	appVersion     = "Other"
	keyringService = "protondriveclient"
)

// Session holds the tokens needed to restore a client across process restarts.
// The key password is NOT stored here — it lives in the OS keyring.
type Session struct {
	Username     string              `json:"username"`
	UID          string              `json:"uid"`
	AccessToken  string              `json:"access_token"`
	RefreshToken string              `json:"refresh_token"`
	PasswordMode proton.PasswordMode `json:"password_mode"`
}

// Login performs a full SRP authentication, optionally completes 2FA, derives
// the salted key password, stores it in the OS keyring, and persists the
// session tokens to disk.
//
// get2FACode is called when TOTP 2FA is required; it should return the code.
// Pass nil only if you are certain the account has no 2FA.
func Login(ctx context.Context, username string, password []byte, get2FACode func() string) (*Session, error) {
	m := newManager()

	c, auth, err := m.NewClientWithLogin(ctx, username, password)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if auth.TwoFA.Enabled&proton.HasTOTP != 0 {
		if get2FACode == nil {
			return nil, fmt.Errorf("account requires 2FA but no code provider was given")
		}
		if err := c.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: get2FACode()}); err != nil {
			return nil, fmt.Errorf("2FA verification failed: %w", err)
		}
	}

	if auth.PasswordMode == proton.TwoPasswordMode {
		return nil, fmt.Errorf("two-password mode is not yet supported")
	}

	// Derive the salted key password while we still have the plaintext password.
	saltedKeyPass, err := deriveKeyPass(ctx, c, password)
	if err != nil {
		return nil, fmt.Errorf("deriving key password: %w", err)
	}

	// Store in the OS keyring (libsecret / GNOME Keyring / KWallet).
	encoded := base64.StdEncoding.EncodeToString(saltedKeyPass)
	if err := keyring.Set(keyringService, username, encoded); err != nil {
		return nil, fmt.Errorf("storing key password in keyring: %w", err)
	}

	return &Session{
		Username:     username,
		UID:          auth.UID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		PasswordMode: auth.PasswordMode,
	}, nil
}

// Unlock restores an API client from stored tokens and unlocks all address
// key rings using the salted key password retrieved from the OS keyring.
// No password prompt is needed.
func (s *Session) Unlock(ctx context.Context) (*proton.Client, *pgpcrypto.KeyRing, error) {
	if s.PasswordMode == proton.TwoPasswordMode {
		return nil, nil, fmt.Errorf("two-password mode is not yet supported")
	}

	encoded, err := keyring.Get(keyringService, s.Username)
	if err != nil {
		return nil, nil, fmt.Errorf("reading key password from keyring: %w\n"+
			"hint: run 'protondrive auth login' to re-authenticate", err)
	}

	saltedKeyPass, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding key password: %w", err)
	}

	m := newManager()
	c := m.NewClient(s.UID, s.AccessToken, s.RefreshToken)

	// Persist any token updates that occur during the mount's lifetime.
	c.AddAuthHandler(func(auth proton.Auth) {
		s.AccessToken = auth.AccessToken
		s.RefreshToken = auth.RefreshToken
		_ = s.Save()
	})

	addrKR, err := unlockWithKeyPass(ctx, c, saltedKeyPass)
	if err != nil {
		c.Close()
		return nil, nil, err
	}

	return c, addrKR, nil
}

// Logout revokes the session on the server, removes the keyring entry, and
// deletes the session file.
func (s *Session) Logout(ctx context.Context) error {
	m := newManager()
	c := m.NewClient(s.UID, s.AccessToken, s.RefreshToken)
	defer c.Close()

	if err := c.AuthDelete(ctx); err != nil {
		return err
	}

	// Best-effort cleanup; don't fail if the keyring entry is already gone.
	_ = keyring.Delete(keyringService, s.Username)
	return nil
}

// Save writes the session to ~/.config/protondriveclient/session.json.
func (s *Session) Save() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(s)
}

// LoadSession reads the stored session, or returns an error if none exists.
func LoadSession() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no stored session — run 'protondrive auth login' first")
		}
		return nil, err
	}
	defer f.Close()
	var s Session
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSession removes the session file from disk.
func DeleteSession() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// deriveKeyPass fetches the user's primary key salt from the API and derives
// the salted mailbox password: bcrypt(password + keySalt).
func deriveKeyPass(ctx context.Context, c *proton.Client, password []byte) ([]byte, error) {
	user, err := c.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	salts, err := c.GetSalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting key salts: %w", err)
	}

	saltedKeyPass, err := salts.SaltForKey(password, user.Keys.Primary().ID)
	if err != nil {
		return nil, fmt.Errorf("salting key password: %w", err)
	}

	return saltedKeyPass, nil
}

// unlockWithKeyPass fetches the user and addresses then unlocks all key rings
// using the already-derived salted key password.
func unlockWithKeyPass(ctx context.Context, c *proton.Client, saltedKeyPass []byte) (*pgpcrypto.KeyRing, error) {
	user, err := c.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	addrs, err := c.GetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting addresses: %w", err)
	}

	_, addrKRs, err := proton.Unlock(user, addrs, saltedKeyPass)
	if err != nil {
		return nil, fmt.Errorf("unlocking keys: %w", err)
	}

	combined, err := pgpcrypto.NewKeyRing(nil)
	if err != nil {
		return nil, err
	}
	for _, kr := range addrKRs {
		for _, k := range kr.GetKeys() {
			if err := combined.AddKey(k); err != nil {
				return nil, err
			}
		}
	}

	if combined.CountEntities() == 0 {
		return nil, fmt.Errorf("no address keys could be unlocked")
	}

	return combined, nil
}

func newManager() *proton.Manager {
	return proton.New(
		proton.WithHostURL(apiBase),
		proton.WithAppVersion(appVersion),
	)
}

func sessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "protondriveclient", "session.json"), nil
}
