package handlers

import (
	"eth2-exporter/db"
	"eth2-exporter/services"
	"eth2-exporter/types"
	"eth2-exporter/utils"
	"eth2-exporter/version"
	"fmt"
	"time"

	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

var userTemplate = template.Must(template.New("user").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/user.html"))
var loginTemplate = template.Must(template.New("login").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/login.html"))
var registerTemplate = template.Must(template.New("register").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/register.html"))
var resetPasswordTemplate = template.Must(template.New("resetPassword").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/resetPassword.html"))
var resendConfirmationTemplate = template.Must(template.New("resetPassword").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/resendConfirmation.html"))
var requestResetPaswordTemplate = template.Must(template.New("resetPassword").Funcs(utils.GetTemplateFuncs()).ParseFiles("templates/layout.html", "templates/requestResetPassword.html"))

var authSessionName = "auth"

// User renders the user-template
func User(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/user",
		},
		Active:                "user",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}
	err := userTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Register handler sends a template that allows for the creation of a new user
func Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/register",
		},
		Active:                "register",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}
	err := registerTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// RegisterPost handles the register-formular to register a new user
func RegisterPost(w http.ResponseWriter, r *http.Request) {
	logger = logger.WithField("route", r.URL.String())
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Errorf("error parsing form: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	pwd := r.FormValue("password")

	if !utils.IsValidEmail(email) {
		session.AddFlash("Error: Invalid email!")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	var existingEmails int
	err = db.FrontendDB.Get(&existingEmails, "SELECT COUNT(*) FROM users WHERE email = $1", email)
	if existingEmails > 0 {
		session.AddFlash("Error: Email already exists!")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	pHash, err := bcrypt.GenerateFromPassword([]byte(pwd), 10)
	if err != nil {
		logger.Errorf("error generating hash for password: %v", err)
		session.AddFlash("Error: Something went wrong, please retry later.")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	registerTs := time.Now().Unix()
	_, err = db.FrontendDB.Exec(`
		INSERT INTO users (password, email, register_ts)
		VALUES ($1, $2, TO_TIMESTAMP($3))`,
		string(pHash), email, registerTs,
	)
	if err != nil {
		logger.Errorf("error saving new user into db: %v", err)
		session.AddFlash("Error: Something went wrong, please retry later.")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	err = sendConfirmationEmail(email)
	if err != nil {
		logger.Errorf("error sending confirmation-email: %v", err)
		session.AddFlash("Error: Something went wrong, we were not able to send an email :/ Please retry later.")
	} else {
		session.AddFlash("Your account has been created! Please verify your email by clicking the link in the email we just sent you.")
	}

	session.Save(r, w)

	http.Redirect(w, r, "/register", http.StatusSeeOther)
}

// Login handler sends a template that allows a user to login
func Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/login",
		},
		Active:                "login",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}
	err := loginTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// LoginPost handles authenticating the user.
func LoginPost(w http.ResponseWriter, r *http.Request) {
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("Error retrieving session for login route: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Errorf("error parsing form: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	pwd := r.FormValue("password")

	up := struct {
		ID       int64
		Email    string
		Password string
	}{}

	err = db.FrontendDB.Get(&up, "SELECT id, email, password FROM users WHERE email = $1", email)
	if err != nil {
		logger.Errorf("error retrieving password for user %v: %v", email, err)
		session.AddFlash("Error: Invalid email or password!")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(up.Password), []byte(pwd))
	if err != nil {
		logger.Errorf("error verifying password for user %v: %v", up.Email, err)
		session.AddFlash("Error: Invalid email or password!")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	session.Values["authenticated"] = true
	session.Values["user_id"] = up.ID
	session.AddFlash("Successfully logged in")

	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles ending the user session.
func Logout(w http.ResponseWriter, r *http.Request) {
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	session.Values["authenticated"] = false
	delete(session.Values, "user_id")
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ResetPassword handler sends a template that lets the user reset his password
func ResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	hash := vars["hash"]

	var userID *int64
	err = db.FrontendDB.Get(&userID, "SELECT id FROM users WHERE password_reset_hash = $1", hash)
	if err != nil {
		logger.Errorf("error resetting password: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/reset", http.StatusSeeOther)
		return
	}

	if userID == nil {
		session.AddFlash("Error: Invalid reset link, please <a href='/requestReset'>retry</a>")
		session.Save(r, w)
		http.Redirect(w, r, "/reset", http.StatusSeeOther)
		return
	}

	user := &types.User{}
	user.Authenticated = true
	user.UserID = *userID

	session.Values["authenticated"] = true
	session.Values["user_id"] = user.UserID

	session.Save(r, w)

	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/register",
		},
		Active:                "register",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}

	err = resetPasswordTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ResetPasswordPost resets the password to the value provided in the form, given that the user is authenticated.
func ResetPasswordPost(w http.ResponseWriter, r *http.Request) {
	logger = logger.WithField("route", r.URL.String())

	user, session, err := getUserSession(w, r)
	if err != nil {
		logger.Errorf("error retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.Authenticated {
		session.AddFlash("Error: You are not authenticated (or did not use the correct reset-link).")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Errorf("error parsing form: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	pwd := r.FormValue("password")
	pHash, err := bcrypt.GenerateFromPassword([]byte(pwd), 10)
	if err != nil {
		logger.Errorf("error generating hash for password: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	_, err = db.FrontendDB.Exec("UPDATE users SET password = $1", pHash)
	if err != nil {
		logger.Errorf("error updating password for user: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	session.Values["authenticated"] = false
	delete(session.Values, "user_id")

	session.AddFlash("Your password has been updated successfully, please log in again!")

	session.Save(r, w)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// RequestResetPassword send a template that lets the user enter his email and request a reset link
func RequestResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/register",
		},
		Active:                "register",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}
	err := requestResetPaswordTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// RequestResetPasswordPost sends a password-reset-link to the provided (via form) email
func RequestResetPasswordPost(w http.ResponseWriter, r *http.Request) {
	logger = logger.WithField("route", r.URL.String())

	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("err retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Errorf("error parsing form: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/requestReset", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")

	if !utils.IsValidEmail(email) {
		session.AddFlash("Error: Invalid email address")
		session.Save(r, w)
		http.Redirect(w, r, "/requestReset", http.StatusSeeOther)
		return
	}

	var exists int
	err = db.FrontendDB.Get(&exists, "SELECT COUNT(*) FROM users WHERE email = $1", email)
	if err != nil {
		logger.Errorf("error retrieving user-count: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/requestReset", http.StatusSeeOther)
		return
	}

	if exists == 0 {
		session.AddFlash("Error: Email does not exist")
		session.Save(r, w)
		http.Redirect(w, r, "/requestReset", http.StatusSeeOther)
		return
	}

	err = sendResetEmail(email)
	if err != nil {
		session.AddFlash("Error: Could not send the email, please retry later.")
	} else {
		session.AddFlash("An email has been sent which contains a link to reset your password.")
	}

	err = session.Save(r, w)
	if err != nil {
		logger.Errorf("error saving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/requestReset", http.StatusSeeOther)
}

// ResendConfirmation handler sends a template for the user to request another confirmation link via email.
func ResendConfirmation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	authData := getAuthData(w, r)
	data := &types.PageData{
		Meta: &types.Meta{
			Description: "beaconcha.in makes the Ethereum 2.0. beacon chain accessible to non-technical end users",
			Path:        "/register",
		},
		Active:                "resendConfirmation",
		Data:                  authData,
		User:                  authData.User,
		Version:               version.Version,
		ChainSlotsPerEpoch:    utils.Config.Chain.SlotsPerEpoch,
		ChainSecondsPerSlot:   utils.Config.Chain.SecondsPerSlot,
		ChainGenesisTimestamp: utils.Config.Chain.GenesisTimestamp,
		CurrentEpoch:          services.LatestEpoch(),
		CurrentSlot:           services.LatestSlot(),
		FinalizationDelay:     services.FinalizationDelay(),
	}
	err := resendConfirmationTemplate.ExecuteTemplate(w, "layout", data)
	if err != nil {
		logger.Errorf("error executing template for %v route: %v", r.URL.String(), err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ResendConfirmationPost handles sending another confirmation email to the user
func ResendConfirmationPost(w http.ResponseWriter, r *http.Request) {
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error retrieving session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Errorf("error parsing form: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/resend", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")

	if !utils.IsValidEmail(email) {
		session.AddFlash("Error: Invalid email!")
		session.Save(r, w)
		http.Redirect(w, r, "/resend", http.StatusSeeOther)
		return
	}

	var exists int
	err = db.FrontendDB.Get("SELECT COUNT(*) FROM users WHERE email = $1", email)
	if err != nil {
		logger.Errorf("error checking if user exists for email-confirmation: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
		session.Save(r, w)
		http.Redirect(w, r, "/resend", http.StatusSeeOther)
		return
	}

	if exists == 0 {
		session.AddFlash("Error: Email does not exist")
		session.Save(r, w)
		http.Redirect(w, r, "/resend", http.StatusSeeOther)
		return
	}

	err = sendConfirmationEmail(email)
	if err != nil {
		logger.Errorf("error sending email-confirmation: %v", err)
		session.AddFlash("Error: Something went wrong :/ Please retry later")
	} else {
		session.AddFlash("Email has been sent")
	}

	session.Save(r, w)
	http.Redirect(w, r, "/resend", http.StatusSeeOther)
}

// ConfirmEmail confirms an users email-address
func ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hash := vars["hash"]
	_, err := db.FrontendDB.Exec("UPDATE users SET email_confirmed = 'TRUE' WHERE email_confirmation_hash = $1", hash)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session, err := utils.SessionStore.Get(r, authSessionName)
	if err == nil {
		session.AddFlash("Your email has been confirmed")
		session.Save(r, w)
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func getUser(w http.ResponseWriter, r *http.Request) *types.User {
	u := &types.User{}
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error getting session from sessionStore: %v", err)
		return u
	}
	ok := false
	u.Authenticated, ok = session.Values["authenticated"].(bool)
	if !ok {
		u.Authenticated = false
		return u
	}
	u.UserID, ok = session.Values["user_id"].(int64)
	if !ok {
		u.Authenticated = false
		return u
	}
	return u
}

func getAuthData(w http.ResponseWriter, r *http.Request) *types.AuthData {
	authData := &types.AuthData{}
	authData.User = &types.User{}
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error getting session from sessionStore: %v", err)
		return authData
	}
	defer session.Save(r, w)
	authData.Flashes = session.Flashes()
	ok := false
	authData.User.Authenticated, ok = session.Values["authenticated"].(bool)
	if !ok {
		authData.User.Authenticated = false
		return authData
	}
	authData.User.UserID, ok = session.Values["user_id"].(int64)
	if !ok {
		authData.User.Authenticated = false
		return authData
	}
	return authData
}

func getUserSession(w http.ResponseWriter, r *http.Request) (*types.User, *sessions.Session, error) {
	u := &types.User{}
	session, err := utils.SessionStore.Get(r, authSessionName)
	if err != nil {
		logger.Errorf("error getting session from sessionStore: %v", err)
		return u, session, err
	}
	ok := false
	u.Authenticated, ok = session.Values["authenticated"].(bool)
	if !ok {
		u.Authenticated = false
		return u, session, nil
	}
	u.UserID, ok = session.Values["user_id"].(int64)
	if !ok {
		u.Authenticated = false
		return u, session, nil
	}
	return u, session, nil
}

func sendConfirmationEmail(email string) error {
	emailConfirmationHashTs := time.Now().Unix()
	emailConfirmationHash := utils.RandomString(40)
	_, err := db.FrontendDB.Exec(`
		UPDATE users 
		SET (email_confirmation_hash, email_confirmation_hash_ts) = ($1, TO_TIMESTAMP($2))
		WHERE email = $3`, emailConfirmationHash, emailConfirmationHashTs, email)
	if err != nil {
		return err
	}

	subject := "beaconcha.in: Verify your email-address"
	msg := fmt.Sprintf(`Please verify your email on https://beaconcha.in by clicking this link:

https://beaconcha.in/confirm/%s

Best regards,

beaconcha.in
`, emailConfirmationHash)
	return utils.SendMail(email, subject, msg)
}

func sendResetEmail(email string) error {
	resetHashTs := time.Now().Unix()
	resetHash := utils.RandomString(40)
	_, err := db.FrontendDB.Exec(`
		UPDATE users 
		SET (password_reset_hash, password_reset_hash_ts) = ($1, TO_TIMESTAMP($2))
		WHERE email = $3`, resetHash, resetHashTs, email)
	if err != nil {
		return err
	}

	subject := "beaconcha.in: Reset your password"
	msg := fmt.Sprintf(`You can reset your password on https://beaconcha.in by clicking this link:

https://beaconcha.in/reset/%s

Best regards,

beaconcha.in
`, resetHash)
	return utils.SendMail(email, subject, msg)
}