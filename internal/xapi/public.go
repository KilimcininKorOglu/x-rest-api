package xapi

// The credential-free read chain. Each tier is tried cheapest first and the
// third-party reader is always last, because it is the only step that leaks the
// queried id or handle off x.com's own hosts. Every tier prefixes its own errors,
// so a joined failure names each tier that was tried.

import (
	"errors"
	"fmt"
)

// PublicTweet reads one tweet with no account: the syndication embed surface
// first, then a guest-token GraphQL read, then FxTwitter.
func (s *Session) PublicTweet(id string) (*Tweet, error) {
	tw, synErr := s.FetchTweetSyndication(id)
	if synErr == nil && tw != nil {
		return tw, nil
	}
	tw, guestErr := s.FetchTweetGuest(id)
	if guestErr == nil && tw != nil {
		return tw, nil
	}
	tw, fxErr := s.FetchTweetPublic(id)
	if fxErr == nil && tw != nil {
		return tw, nil
	}
	return nil, errors.Join(synErr, guestErr, fxErr,
		fmt.Errorf("tweet %s: no credential-free tier returned a result", id))
}

// PublicUser reads a profile with no account: a guest-token GraphQL read first,
// then FxTwitter. The syndication surface serves profiles as an HTML page rather
// than JSON, so it is not part of this chain.
func (s *Session) PublicUser(handle string) (*XUser, error) {
	u, guestErr := s.FetchUserGuest(handle)
	if guestErr == nil && u != nil {
		return u, nil
	}
	u, fxErr := s.FetchUserPublic(handle)
	if fxErr == nil && u != nil {
		return u, nil
	}
	return nil, errors.Join(guestErr, fxErr,
		fmt.Errorf("user %s: no credential-free tier returned a result", handle))
}
