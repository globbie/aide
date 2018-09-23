package identity_provider

type IdentityProvider interface {
	Login()
	Logout()
	Register()
}
