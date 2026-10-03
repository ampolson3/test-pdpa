package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/files"
)

type fakeSettingsFiles struct {
	file     files.File
	attached map[uuid.UUID]uuid.UUID
}

func (f *fakeSettingsFiles) Get(_ context.Context, id uuid.UUID) (files.File, error) {
	if id != f.file.ID {
		return files.File{}, files.ErrNotFound
	}
	return f.file, nil
}

func (f *fakeSettingsFiles) Attach(_ context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	if entityType != orgservice.OrgSettingsEntityType {
		return files.ErrUnknownEntity
	}
	f.attached[id] = entityID
	return nil
}

func TestOrgSettings_DefaultsAndAcceptance(t *testing.T) {
	e := setup(t, "orgset")
	e.in(t, func(ctx context.Context) error {
		s, err := e.svc.GetOrgSettings(ctx)
		if err != nil {
			return err
		}
		if s.DefaultLanguage != "th" || s.DateEra != "BE" || s.RowVersion != 0 {
			t.Errorf("a tenant with no saved settings should read as the defaults: %+v", s)
		}
		return nil
	})

	logo := &fakeSettingsFiles{file: files.File{ID: uuid.New(), MimeType: "image/png", AVStatus: "clean"}, attached: map[uuid.UUID]uuid.UUID{}}
	e.svc.Files = logo
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.SaveOrgSettings(ctx, orgservice.OrgSettings{
			DefaultLanguage: "en", DateEra: "CE",
			Branding: orgservice.Branding{LogoFileID: &logo.file.ID, ThemeColor: "#0B5FFF"},
		}, 0)
		if err != nil {
			return err
		}
		if out.DefaultLanguage != "en" || out.DateEra != "CE" || out.RowVersion != 1 || out.Branding.ThemeColor != "#0B5FFF" {
			t.Errorf("save did not stick: %+v", out)
		}
		if logo.attached[logo.file.ID] == uuid.Nil {
			t.Error("logo should have been attached")
		}

		// The version everyone reads now reflects the save (acceptance: read after write is the same tenant's data).
		got, err := e.svc.GetOrgSettings(ctx)
		if err != nil {
			return err
		}
		if got.DefaultLanguage != "en" || got.RowVersion != 1 {
			t.Errorf("read back mismatch: %+v", got)
		}

		// A stale version is refused.
		if _, err := e.svc.SaveOrgSettings(ctx, orgservice.OrgSettings{DefaultLanguage: "th", DateEra: "BE"}, 0); !errors.Is(err, orgservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

func TestOrgSettings_Validation(t *testing.T) {
	e := setup(t, "orgsetv")
	e.in(t, func(ctx context.Context) error {
		for name, in := range map[string]orgservice.OrgSettings{
			"bad language":     {DefaultLanguage: "fr", DateEra: "BE"},
			"bad era":          {DefaultLanguage: "th", DateEra: "XX"},
			"bad theme color":  {DefaultLanguage: "th", DateEra: "BE", Branding: orgservice.Branding{ThemeColor: "blue"}},
			"bad accent color": {DefaultLanguage: "th", DateEra: "BE", Branding: orgservice.Branding{AccentColor: "#ZZZZZZ"}},
		} {
			if _, err := e.svc.SaveOrgSettings(ctx, in, 0); !errors.Is(err, orgservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})
}

func TestOrgSettings_LogoMustBeTheCallersCleanUpload(t *testing.T) {
	e := setup(t, "orgsetlogo")
	other := &fakeSettingsFiles{file: files.File{ID: uuid.New(), MimeType: "application/pdf", AVStatus: "clean"}, attached: map[uuid.UUID]uuid.UUID{}}
	e.svc.Files = other
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveOrgSettings(ctx, orgservice.OrgSettings{DefaultLanguage: "th", DateEra: "BE",
			Branding: orgservice.Branding{LogoFileID: &other.file.ID}}, 0); !errors.Is(err, orgservice.ErrLogoNotUse) {
			t.Errorf("non-image logo: %v, want ErrLogoNotUse", err)
		}
		return nil
	})
}

func TestOrgSettings_Isolation(t *testing.T) {
	a := setup(t, "orgseta")
	b := setup(t, "orgsetb")
	a.in(t, func(ctx context.Context) error {
		_, err := a.svc.SaveOrgSettings(ctx, orgservice.OrgSettings{DefaultLanguage: "en", DateEra: "CE"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		s, err := b.svc.GetOrgSettings(ctx)
		if err != nil {
			return err
		}
		if s.DefaultLanguage != "th" || s.RowVersion != 0 {
			t.Errorf("tenant B should not see tenant A's settings: %+v", s)
		}
		return nil
	})
}
