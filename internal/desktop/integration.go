package desktop

import (
	"errors"

	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/Oranjekop/TextViewer/locales"
)

type integrationState struct {
	Associations bool
	ContextMenu  bool
}

type integrationBackend interface {
	State() (integrationState, error)
	Associations(bool) error
	ContextMenu(enabled, trust bool) error
	Cleanup() error
}

type windowsIntegration struct{}

func (windowsIntegration) State() (integrationState, error) {
	associations, err := fileassoc.Registered()
	if err != nil {
		return integrationState{}, err
	}
	menu, err := shellmenu.Installed()
	return integrationState{associations, menu}, err
}

func (windowsIntegration) Associations(enabled bool) error {
	if enabled {
		return fileassoc.Register()
	}
	return fileassoc.RemoveRegistration()
}

func (windowsIntegration) ContextMenu(enabled, trust bool) error {
	if !enabled {
		return shellmenu.Uninstall()
	}
	if trust {
		return shellmenu.TrustAndInstall()
	}
	return shellmenu.Install()
}

func (windowsIntegration) Cleanup() error { return shellmenu.Cleanup() }

type integrationOptions struct {
	trust          bool
	refreshEnabled bool
}

func applyIntegrations(backend integrationBackend, desired, current integrationState, options integrationOptions) error {
	if desired.Associations != current.Associations || options.refreshEnabled && desired.Associations {
		if err := backend.Associations(desired.Associations); err != nil {
			return locales.WrapError(err, locales.AssociationsFailed)
		}
	}
	if desired.ContextMenu != current.ContextMenu || options.refreshEnabled && desired.ContextMenu {
		if err := backend.ContextMenu(desired.ContextMenu, options.trust); err != nil {
			if desired.Associations != current.Associations {
				if rollback := backend.Associations(current.Associations); rollback != nil {
					err = errors.Join(err, locales.WrapError(rollback, locales.AssociationsRollbackFailed))
				}
			}
			return locales.WrapError(err, locales.ContextMenuFailed)
		}
	}
	return nil
}

func desiredIntegrations(p preferences, current integrationState) integrationState {
	if p.FileAssociations != nil {
		current.Associations = *p.FileAssociations
	}
	if p.ContextMenu != nil {
		current.ContextMenu = *p.ContextMenu
	}
	return current
}

// ConfigureInstalledIntegration preserves existing opt-ins on upgrade. A new
// installation waits for the user to enable integrations in Settings.
func ConfigureInstalledIntegration() error {
	return configureInstalledIntegration(windowsIntegration{}, loadPreferences())
}

func configureInstalledIntegration(backend integrationBackend, p preferences) error {
	current, err := backend.State()
	if err != nil {
		return err
	}
	// Refresh enabled entries on upgrade to bind them to the new executable.
	if err := applyIntegrations(backend, desiredIntegrations(p, current), current, integrationOptions{refreshEnabled: true}); err != nil {
		return err
	}
	return backend.Cleanup()
}
