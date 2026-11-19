//go:build linux && cgo

package apps

/*
#cgo pkg-config: gdk-pixbuf-2.0
#include <gdk-pixbuf/gdk-pixbuf.h>
#include <stdlib.h>

static char *raster_icon(const char *path, gsize *len) {
  GdkPixbuf *icon = gdk_pixbuf_new_from_file_at_scale(path, 32, 32, TRUE, NULL);
  if (!icon) return NULL;
  char *png = NULL;
  gboolean ok = gdk_pixbuf_save_to_buffer(icon, &png, len, "png", NULL, NULL);
  g_object_unref(icon);
  if (!ok) { g_free(png); return NULL; }
  return png;
}
*/
import "C"
import "unsafe"

func rasterIcon(path string) []byte {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var size C.gsize
	b := C.raster_icon(p, &size)
	if b == nil {
		return nil
	}
	defer C.g_free(C.gpointer(unsafe.Pointer(b)))
	return C.GoBytes(unsafe.Pointer(b), C.int(size))
}
