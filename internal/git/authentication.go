package git

// Credentials holds user credentials to authenticate and authorize while
// communicating with remote if required
type Credentials struct {
	// User is the user id for authentication
	User string
	// Password is the secret information required for authentication
	Password string
}
