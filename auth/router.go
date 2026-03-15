package auth

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/panyam/goutils/utils"
)

type contextKey string

const loggedInUserKey contextKey = "loggedInUser"

type AuthConfig struct {
	SessionGetter      func(r *http.Request, param string) any
	CallbackURLParam   string
	DefaultRedirectURL string
	GetRedirURL        func(r *http.Request) string
	UserParamName      string
	NoLoginRedirect    bool
}

/**
 * Ensures that config values have reasonable defaults.
 */
func (a *AuthConfig) EnsureReasonableDefaults() {
	if a.UserParamName == "" {
		a.UserParamName = "loggedInUserId"
	}
	if a.CallbackURLParam == "" {
		a.CallbackURLParam = "/callbackURL"
	}
	if a.DefaultRedirectURL == "" {
		a.DefaultRedirectURL = "/login"
	}
	if a.GetRedirURL == nil && !a.NoLoginRedirect {
		a.GetRedirURL = func(r *http.Request) string { return a.DefaultRedirectURL }
	}
}

func GetLoggedInUser(ctx context.Context) string {
	if v, ok := ctx.Value(loggedInUserKey).(string); ok {
		return v
	}
	return ""
}

func SetLoggedInUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, loggedInUserKey, userID)
}

func (a *AuthConfig) GetLoggedInUserId(r *http.Request) string {
	if uid := GetLoggedInUser(r.Context()); uid != "" {
		return uid
	}
	userParam := a.SessionGetter(r, a.UserParamName)
	if userParam != "" && userParam != nil {
		return userParam.(string)
	}
	return ""
}

/**
 * Extracts user info from the request and saves it into current user.
 * Can be used by further middleware down the line to get the request's
 * user info
 */
func (a *AuthConfig) ExtractUserInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userParam := a.SessionGetter(r, a.UserParamName)
		if userParam != "" && userParam != nil {
			ctx := SetLoggedInUser(r.Context(), userParam.(string))
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

/**
 * Redirects users to login screen if they are not logged in
 */
func (a *AuthConfig) EnsureLoginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if a.EnsureLogin(w, r) {
				next.ServeHTTP(w, r)
			}
		},
	)
}

func (a *AuthConfig) EnsureLogin(w http.ResponseWriter, r *http.Request) bool {
	a.EnsureReasonableDefaults()
	userParam := a.SessionGetter(r, a.UserParamName)
	if userParam == "" || userParam == nil {
		redirUrl := ""
		if a.GetRedirURL != nil {
			redirUrl = a.GetRedirURL(r)
		}
		if redirUrl != "" {
			originalUrl := r.URL.Path
			encodedUrl := utils.EncodeURIComponent(originalUrl)
			fullRedirUrl := fmt.Sprintf("%s?%s=%s", redirUrl, a.CallbackURLParam, encodedUrl)
			http.Redirect(w, r, fullRedirUrl, http.StatusFound)
		} else {
			http.Error(w, "Login Failed", http.StatusUnauthorized)
		}
	} else {
		log.Println("Setting Logged In User Id: ", userParam)
		ctx := SetLoggedInUser(r.Context(), userParam.(string))
		r = r.WithContext(ctx)
		return true
	}
	return false
}
