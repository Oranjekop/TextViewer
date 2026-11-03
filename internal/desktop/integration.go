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

func applyIntegrations(backend integrationBackend, desired, current integrationState, trust bool) error {
	// Reapply enabled entries so an upgrade also repairs stale executable paths.
	if desired.Associations || current.Associations {
		if err := backend.Associations(desired.Associations); err != nil {
			return locales.WrapError(err, locales.AssociationsFailed)
		}
	}
	if desired.ContextMenu || current.ContextMenu {
		if err := backend.ContextMenu(desired.ContextMenu, trust); err != nil {
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
	if err := applyIntegrations(backend, desiredIntegrations(p, current), current, false); err != nil {
		return err
	}
	return backend.Cleanup()
}
