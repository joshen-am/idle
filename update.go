package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Actualizaciones automáticas: al abrir, la app mira en GitHub si hay una versión nueva,
// la descarga, se reemplaza a sí misma y se vuelve a abrir. Nadie tiene que descargar nada.
// version y updateRepo se fijan al fabricar la app (build.sh).
var (
	version    = "dev"
	updateRepo = "" // "usuario/repositorio" en GitHub
)

type Updater struct {
	mu     sync.Mutex
	state  string // "", "descargando", "listo", "error"
	newVer string
	apply  func() error // reemplaza la app y la vuelve a abrir
}

func (u *Updater) Status() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state, u.newVer
}

func (u *Updater) set(state, ver string) {
	u.mu.Lock()
	u.state, u.newVer = state, ver
	u.mu.Unlock()
}

func newerThan(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func assetName() string {
	if runtime.GOOS == "darwin" {
		return "MiniJuegos-Mac.zip"
	}
	return "MiniJuegos-Windows.zip"
}

// Check corre en segundo plano al abrir la app.
func (u *Updater) Check() {
	cleanupOld()
	api := os.Getenv("MJ_UPDATE_API") // para pruebas
	if api == "" {
		if updateRepo == "" || version == "dev" {
			return
		}
		api = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" && os.Getenv("MJ_UPDATE_API") == "" {
		return
	}
	cl := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest("GET", api, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MiniJuegos/"+version)
	res, err := cl.Do(req)
	if err != nil {
		logf("actualizar: no se pudo consultar: %v", err)
		return
	}
	defer res.Body.Close()
	var rel struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&rel) != nil {
		logf("actualizar: respuesta %d", res.StatusCode)
		return
	}
	if !newerThan(rel.Tag, version) {
		return
	}
	url := ""
	for _, a := range rel.Assets {
		if a.Name == assetName() {
			url = a.URL
		}
	}
	if url == "" {
		return
	}
	newVer := strings.TrimPrefix(rel.Tag, "v")
	logf("actualizar: %s -> %s", version, newVer)
	u.set("descargando", newVer)
	apply, err := prepare(cl, url)
	if err != nil {
		logf("actualizar: error %v", err)
		u.set("error", newVer)
		return
	}
	u.mu.Lock()
	u.state, u.apply = "listo", apply
	u.mu.Unlock()
}

// Apply cambia a la versión nueva y la abre. Después hay que cerrar esta.
func (u *Updater) Apply() bool {
	u.mu.Lock()
	apply := u.apply
	u.apply = nil
	u.mu.Unlock()
	if apply == nil {
		return false
	}
	if err := apply(); err != nil {
		logf("actualizar: no se pudo aplicar: %v", err)
		u.set("error", u.newVer)
		return false
	}
	return true
}

// prepare descarga y descomprime la versión nueva al lado de la actual (mismo disco).
func prepare(cl *http.Client, url string) (func() error, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	res, err := cl.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("descarga %d", res.StatusCode)
	}

	target := exe // en Windows se reemplaza el .exe
	if runtime.GOOS == "darwin" {
		target = filepath.Dir(filepath.Dir(filepath.Dir(exe))) // .../Mini Juegos.app
		if !strings.HasSuffix(target, ".app") {
			return nil, fmt.Errorf("no está dentro de una .app: %s", exe)
		}
	}
	dir := filepath.Dir(target)
	tmpZip, err := os.CreateTemp(dir, ".mj-update-*.zip")
	if err != nil {
		return nil, fmt.Errorf("sin permiso para escribir en %s: %v", dir, err)
	}
	defer os.Remove(tmpZip.Name())
	if _, err := io.Copy(tmpZip, res.Body); err != nil {
		tmpZip.Close()
		return nil, err
	}
	tmpZip.Close()
	stage, err := os.MkdirTemp(dir, ".mj-update-")
	if err != nil {
		return nil, err
	}
	if err := unzip(tmpZip.Name(), stage); err != nil {
		os.RemoveAll(stage)
		return nil, err
	}

	if runtime.GOOS == "darwin" {
		newApp := filepath.Join(stage, filepath.Base(target))
		if _, err := os.Stat(newApp); err != nil {
			newApp = filepath.Join(stage, "Mini Juegos.app")
		}
		return func() error {
			old := target + ".old"
			os.RemoveAll(old)
			if err := os.Rename(target, old); err != nil {
				return err
			}
			if err := os.Rename(newApp, target); err != nil {
				os.Rename(old, target)
				return err
			}
			os.RemoveAll(old)
			os.RemoveAll(stage)
			return exec.Command("open", "-n", target).Start()
		}, nil
	}
	newExe := filepath.Join(stage, filepath.Base(target))
	if _, err := os.Stat(newExe); err != nil {
		newExe = filepath.Join(stage, "MiniJuegos.exe")
	}
	return func() error {
		old := target + ".old"
		os.Remove(old)
		if err := os.Rename(target, old); err != nil { // un .exe abierto se puede renombrar, no borrar
			return err
		}
		if err := os.Rename(newExe, target); err != nil {
			os.Rename(old, target)
			return err
		}
		os.RemoveAll(stage)
		return exec.Command(target).Start()
	}, nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		p := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(p, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("ruta inválida en el zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o200)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// cleanupOld borra restos de una actualización anterior.
func cleanupOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
	dir := filepath.Dir(exe)
	if runtime.GOOS == "darwin" {
		dir = filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".mj-update-*"))
	for _, m := range matches {
		os.RemoveAll(m)
	}
}
