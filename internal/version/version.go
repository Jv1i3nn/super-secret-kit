package version

// Version est le numéro de version de l'application
// Cette valeur est injectée au moment du build via ldflags
var Version = "dev"

// GetVersion retourne la version actuelle de l'application
func GetVersion() string {
	return Version
}
