package catalog

import "context"

// AccountAvatar reads only the active account's display image. It is separate
// from membership verification: an unavailable photo must not block playback.
func (c *Client) AccountAvatar(ctx context.Context) (string, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return "", err
	}
	if scope.session == nil {
		return "", ErrAuthenticationRequired
	}
	response, err := c.apiRequest(ctx, "account/account_menu", map[string]any{}, "")
	if err != nil {
		return "", err
	}
	avatar, err := parseAccountAvatar(response)
	if scopeErr := c.scopeError(scope); scopeErr != nil {
		return "", scopeErr
	}
	return avatar, err
}

func parseAccountAvatar(response map[string]any) (string, error) {
	for _, action := range arrayAt(response, "actions") {
		header, ok := at(action, "openPopupAction", "popup", "multiPageMenuRenderer", "header", "activeAccountHeaderRenderer").(map[string]any)
		if ok {
			// Do not search other menu images: those may belong to another
			// account or channel offered by the account switcher.
			return artwork(map[string]any{"thumbnail": header["accountPhoto"]}), nil
		}
	}
	return "", ErrInvalidResponse
}
