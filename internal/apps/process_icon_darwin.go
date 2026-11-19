//go:build darwin && cgo

package apps

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

static void *netpilot_icon(const char *path, int *length) {
  @autoreleasepool {
    NSString *exe = [NSString stringWithUTF8String:path];
    if (!exe) return NULL;
    NSRange bundle = [exe rangeOfString:@".app/Contents/"];
    if (bundle.location == NSNotFound) return NULL;
    NSString *app = [exe substringToIndex:bundle.location + 4];
    NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:app];
    NSBitmapImageRep *bitmap = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:32 pixelsHigh:32 bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
    NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:bitmap];
    [NSGraphicsContext saveGraphicsState];
    [NSGraphicsContext setCurrentContext:ctx];
    [icon drawInRect:NSMakeRect(0,0,32,32) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1];
    [NSGraphicsContext restoreGraphicsState];
    NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
    [bitmap release];
    if (!png.length) return NULL;
    void *result = malloc(png.length);
    if (!result) return NULL;
    memcpy(result, png.bytes, png.length);
    *length = (int)png.length;
    return result;
  }
}
*/
import "C"
import "unsafe"

func nativeIcon(executable string) []byte {
	path := C.CString(executable)
	defer C.free(unsafe.Pointer(path))
	var n C.int
	b := C.netpilot_icon(path, &n)
	if b == nil {
		return nil
	}
	defer C.free(b)
	return C.GoBytes(b, n)
}
