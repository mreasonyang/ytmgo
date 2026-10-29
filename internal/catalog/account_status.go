package catalog

import "context"

// AccountStatus comes only from the service's explicit WEB_REMIX feedback.
// Missing or unrecognized state is an error, never an anonymous/free default.
type AccountStatus struct {
	LoggedIn     bool
	MusicPremium bool
}

func (c *Client) AccountStatus(ctx context.Context) (AccountStatus, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return AccountStatus{}, err
	}
	response, err := c.apiRequestWithOptions(ctx, "browse", map[string]any{
		"browseId": "MPREb_4pL8gzRtw1p",
	}, requestOptions{allowLoggedOut: true})
	if err != nil {
		return AccountStatus{}, err
	}
	status, err := parseAccountStatus(response)
	if scopeErr := c.scopeError(scope); scopeErr != nil {
		return AccountStatus{}, scopeErr
	}
	return status, err
}

func parseAccountStatus(response map[string]any) (AccountStatus, error) {
	tracking, ok := at(response, "responseContext", "serviceTrackingParams").([]any)
	if !ok {
		return AccountStatus{}, ErrAccountStatusUnavailable
	}
	var loggedIn, premium bool
	var hasLogin, hasPremium bool
	for _, raw := range tracking {
		entry, ok := raw.(map[string]any)
		if !ok || entry["service"] != "GFEEDBACK" {
			continue
		}
		params, ok := entry["params"].([]any)
		if !ok {
			return AccountStatus{}, ErrAccountStatusUnavailable
		}
		for _, rawParam := range params {
			param, ok := rawParam.(map[string]any)
			if !ok {
				return AccountStatus{}, ErrAccountStatusUnavailable
			}
			switch param["key"] {
			case "logged_in":
				value, ok := param["value"].(string)
				if !ok || value != "0" && value != "1" {
					return AccountStatus{}, ErrAccountStatusUnavailable
				}
				current := value == "1"
				if hasLogin && current != loggedIn {
					return AccountStatus{}, ErrAccountStatusUnavailable
				}
				loggedIn, hasLogin = current, true
			case "has_unlimited_entitlement":
				value, ok := param["value"].(string)
				if !ok || value != "True" && value != "False" {
					return AccountStatus{}, ErrAccountStatusUnavailable
				}
				current := value == "True"
				if hasPremium && current != premium {
					return AccountStatus{}, ErrAccountStatusUnavailable
				}
				premium, hasPremium = current, true
			}
		}
	}
	if !hasLogin || !hasPremium || premium && !loggedIn {
		return AccountStatus{}, ErrAccountStatusUnavailable
	}
	return AccountStatus{LoggedIn: loggedIn, MusicPremium: premium}, nil
}
