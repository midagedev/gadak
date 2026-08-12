//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// The one UI delegate every embedded page shares. Atlassian is full of
// target="_blank"; with no delegate those clicks die (the same silence the
// main webview had before /desktop/open). In an embedded pane a new window
// makes no sense, so the request loads in place instead.
@interface ScryEmbedUIDelegate : NSObject <WKUIDelegate>
@end
@implementation ScryEmbedUIDelegate
- (WKWebView *)webView:(WKWebView *)webView
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
               forNavigationAction:(WKNavigationAction *)navigationAction
                    windowFeatures:(WKWindowFeatures *)windowFeatures {
	if (navigationAction.request.URL != nil) {
		[webView loadRequest:navigationAction.request];
	}
	return nil;
}
@end

static ScryEmbedUIDelegate *embedUIDelegate(void) {
	static ScryEmbedUIDelegate *d = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{ d = [[ScryEmbedUIDelegate alloc] init]; });
	return d;
}

// Frames arrive in the SPA's coordinate space: CSS px with y measured from the
// top. The main webview fills the contentView 1:1 (points == CSS px), so only
// the y axis needs flipping into AppKit's bottom-left origin.
static NSRect embedRect(NSView *content, double x, double y, double w, double h) {
	double ch = content.bounds.size.height;
	return NSMakeRect(x, ch - y - h, w, h);
}

// embedCreate builds a hidden WKWebView over the app's webview and starts the
// load. Returned +1 retained; embedClose releases. The width/height autoresize
// mask keeps all four margins pinned, so live window resizes track natively
// between the SPA's layout reports.
static void *embedCreate(void *nsWindow, const char *url,
                         double x, double y, double w, double h) {
	NSString *s = [NSString stringWithUTF8String:url];
	__block WKWebView *wv = nil;
	dispatch_sync(dispatch_get_main_queue(), ^{
		NSWindow *win = (NSWindow *)nsWindow;
		NSView *content = [win contentView];
		WKWebViewConfiguration *config = [[[WKWebViewConfiguration alloc] init] autorelease];
		wv = [[WKWebView alloc] initWithFrame:embedRect(content, x, y, w, h)
		                        configuration:config];
		wv.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		wv.UIDelegate = embedUIDelegate();
		wv.hidden = YES;
		[content addSubview:wv];
		NSURL *u = [NSURL URLWithString:s];
		if (u != nil) {
			[wv loadRequest:[NSURLRequest requestWithURL:u]];
		}
	});
	return wv;
}

static void embedSetFrame(void *nsWindow, void *webView,
                          double x, double y, double w, double h) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *win = (NSWindow *)nsWindow;
		WKWebView *wv = (WKWebView *)webView;
		wv.frame = embedRect([win contentView], x, y, w, h);
	});
}

static void embedSetHidden(void *webView, bool hidden) {
	dispatch_async(dispatch_get_main_queue(), ^{
		((WKWebView *)webView).hidden = hidden ? YES : NO;
	});
}

static void embedClose(void *webView) {
	dispatch_async(dispatch_get_main_queue(), ^{
		WKWebView *wv = (WKWebView *)webView;
		[wv removeFromSuperview];
		[wv release];
	});
}

// embedInfo copies title/URL out under the main queue for the state poll.
// Caller frees both.
static void embedInfo(void *webView, char **title, char **url) {
	__block char *t = NULL;
	__block char *u = NULL;
	dispatch_sync(dispatch_get_main_queue(), ^{
		WKWebView *wv = (WKWebView *)webView;
		if (wv.title != nil) {
			t = strdup([wv.title UTF8String]);
		}
		if (wv.URL != nil) {
			u = strdup([wv.URL.absoluteString UTF8String]);
		}
	});
	*title = t;
	*url = u;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// macEmbedder drives WKWebViews layered over the main window's webview — the
// native half of the in-app browser pane. All Cocoa work happens on the main
// queue inside the C shims; methods here are safe from handler goroutines
// (the Wails asset server does not call them on the main thread — dispatch_sync
// would deadlock there, and the /desktop/open + window-spawn routes already
// rely on the same fact).
type macEmbedder struct {
	window func() unsafe.Pointer // main window's NSWindow, resolved lazily
}

func newPlatformEmbedder(window func() unsafe.Pointer) embedder {
	return &macEmbedder{window: window}
}

func (m *macEmbedder) Create(url string, f frameRect) (unsafe.Pointer, error) {
	win := m.window()
	if win == nil {
		return nil, errors.New("main window not ready")
	}
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	wv := C.embedCreate(win, cu, C.double(f.X), C.double(f.Y), C.double(f.W), C.double(f.H))
	if wv == nil {
		return nil, errors.New("embed create failed")
	}
	return wv, nil
}

func (m *macEmbedder) SetFrame(wv unsafe.Pointer, f frameRect) {
	win := m.window()
	if win == nil {
		return
	}
	C.embedSetFrame(win, wv, C.double(f.X), C.double(f.Y), C.double(f.W), C.double(f.H))
}

func (m *macEmbedder) SetHidden(wv unsafe.Pointer, hidden bool) {
	C.embedSetHidden(wv, C.bool(hidden))
}

func (m *macEmbedder) Close(wv unsafe.Pointer) {
	C.embedClose(wv)
}

func (m *macEmbedder) Info(wv unsafe.Pointer) (title, url string) {
	var ct, cu *C.char
	C.embedInfo(wv, &ct, &cu)
	if ct != nil {
		title = C.GoString(ct)
		C.free(unsafe.Pointer(ct))
	}
	if cu != nil {
		url = C.GoString(cu)
		C.free(unsafe.Pointer(cu))
	}
	return title, url
}
