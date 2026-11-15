package explorer

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

type folderDirectLinkPage struct {
	SiteName string
	Folder   string
	Parent   string
	HasUp    bool
	Entries  []folderDirectLinkPageEntry
}

type folderDirectLinkPageEntry struct {
	Name     string
	Url      string
	Size     string
	Folder   bool
	Updated  string
	IconText string
}

const folderDirectLinkPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>{{.Folder}} - {{.SiteName}}</title>
<style>
:root{color-scheme:light dark;--bg:#f7f8fa;--card:#fff;--fg:#1f2329;--muted:#6b7280;--line:#e5e7eb;--accent:#2563eb}
@media (prefers-color-scheme:dark){:root{--bg:#111418;--card:#181c21;--fg:#e6e8eb;--muted:#9aa3ad;--line:#2a2f36;--accent:#60a5fa}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"Noto Sans",sans-serif}
.wrap{max-width:920px;margin:0 auto;padding:28px 16px 48px}
h1{font-size:20px;font-weight:600;margin:0 0 4px;word-break:break-all}
.meta{color:var(--muted);font-size:12px;margin-bottom:18px}
.list{background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden}
.row{display:flex;align-items:center;gap:12px;padding:11px 14px;border-top:1px solid var(--line);text-decoration:none;color:inherit}
.row:first-child{border-top:0}
.row:hover{background:rgba(37,99,235,.07)}
.icon{width:26px;height:26px;border-radius:6px;display:flex;align-items:center;justify-content:center;font-size:11px;font-weight:700;flex:0 0 auto;background:rgba(37,99,235,.12);color:var(--accent)}
.icon.folder{background:rgba(234,179,8,.16);color:#b45309}
.name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.size{color:var(--muted);font-size:12px;white-space:nowrap}
.updated{color:var(--muted);font-size:12px;white-space:nowrap;width:150px;text-align:right}
@media (max-width:640px){.updated{display:none}.wrap{padding-top:18px}}
.empty{padding:36px 16px;text-align:center;color:var(--muted)}
</style>
</head>
<body>
<div class="wrap">
  <h1>{{.Folder}}</h1>
  <div class="meta">{{.SiteName}} &middot; {{len .Entries}} item(s)</div>
  <div class="list">
    {{if .HasUp}}<a class="row" href="{{.Parent}}"><span class="icon folder">..</span><span class="name">..</span></a>{{end}}
    {{range .Entries}}
    <a class="row" href="{{.Url}}">
      <span class="icon{{if .Folder}} folder{{end}}">{{.IconText}}</span>
      <span class="name">{{.Name}}</span>
      <span class="size">{{.Size}}</span>
      <span class="updated">{{.Updated}}</span>
    </a>
    {{else}}
    <div class="empty">This folder is empty.</div>
    {{end}}
  </div>
</div>
</body>
</html>`

var folderDirectLinkTemplate = template.Must(template.New("folder-direct-link").Parse(folderDirectLinkPageTemplate))

// RedirectFolderDirectLink renders a browse page for a folder direct link.
func RedirectFolderDirectLink(c *gin.Context, name string) error {
	dep := dependency.FromContext(c)
	settings := dep.SettingProvider()

	sourceLinkID := hashid.FromContext(c)
	ctx := context.WithValue(c, inventory.LoadDirectLinkFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadFileEntity{}, true)
	ctx = context.WithValue(ctx, inventory.LoadFileUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserGroup{}, true)
	dl, err := dep.DirectLinkClient().GetByNameID(ctx, sourceLinkID, name)
	if err != nil {
		return serializer.NewError(serializer.CodeNotFound, "direct link not found", err)
	}

	m := manager.NewFileManager(dep, dl.Edges.File.Edges.Owner)
	defer m.Recycle()

	// The listing needs entity policy information to build child download URLs.
	managerCtx := context.WithValue(ctx, inventory.LoadFileEntity{}, true)
	listing, err := m.GetFolderDirectLinkEntries(managerCtx, dl)
	if err != nil {
		return err
	}

	page := folderDirectLinkPage{
		SiteName: firstNonEmpty(settings.SiteBasic(c).Name, "Cloudreve"),
		Folder:   listing.Folder.Name(),
	}
	parentUri := listing.Folder.Uri(false).DirUri()
	if parentUri != nil && !listing.Folder.IsRootFolder() {
		parentLink, parentErr := m.GetDirectLink(
			context.WithValue(ctx, manager.DirectLinkReuseCtx{}, true), parentUri)
		if parentErr == nil && len(parentLink) > 0 {
			page.Parent = parentLink[0].Url
			page.HasUp = true
		}
	}

	for _, entry := range listing.Entries {
		icon := strings.ToUpper(strings.TrimPrefix(extension(entry.File.Name()), "."))
		if icon == "" {
			icon = "FILE"
		}
		if len(icon) > 4 {
			icon = icon[:4]
		}
		if entry.Folder {
			icon = "DIR"
		}

		page.Entries = append(page.Entries, folderDirectLinkPageEntry{
			Name:     entry.File.Name(),
			Url:      entry.Url,
			Size:     formatDirectLinkSize(entry.File, entry.Folder),
			Folder:   entry.Folder,
			Updated:  entry.File.UpdatedAt().Local().Format("2006-01-02 15:04"),
			IconText: icon,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	return folderDirectLinkTemplate.Execute(c.Writer, page)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func formatDirectLinkSize(file fs.File, folder bool) string {
	if folder {
		return ""
	}
	return humanSize(file.Size())
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func extension(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return ""
	}
	return name[idx:]
}
