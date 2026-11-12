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
	Associations(bool, []string) error
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

func (windowsIntegration) Associations(enabled bool, extensions []string) error {
	if enabled {
		return fileassoc.RegisterSelection(extensions)
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
	trust              bool
	refreshEnabled     bool
	selectionChanged   bool
	extensions         []string
	previousExtensions []string
}

func applyIntegrations(backend integrationBackend, desired, current integrationState, options integrationOptions) error {
	associationsChanged := desired.Associations != current.Associations || desired.Associations && (options.refreshEnabled || options.selectionChanged)
	if associationsChanged {
		if err := backend.Associations(desired.Associations, options.extensions); err != nil {
			return locales.WrapError(err, locales.AssociationsFailed)
		}
	}
	if desired.ContextMenu != current.ContextMenu || options.refreshEnabled && desired.ContextMenu {
		if err := backend.ContextMenu(desired.ContextMenu, options.trust); err != nil {
			if associationsChanged {
				if rollback := backend.Associations(current.Associations, options.previousExtensions); rollback != nil {
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
	if err := applyIntegrations(backend, desiredIntegrations(p, current), current, integrationOptions{refreshEnabled: true, extensions: p.AssociationExtensions, previousExtensions: p.AssociationExtensions}); err != nil {
		return err
	}
	return backend.Cleanup()
}
