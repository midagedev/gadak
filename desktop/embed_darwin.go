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
// makes no sense, so the request loads in place instead. This whole file is
// a workaround for wails v3 having no child/overlay webview API — upstream
// tracker wailsapp/wails#1997 (open implementation attempt: PR #5008).
@interface GadakEmbedUIDelegate : NSObject <WKUIDelegate>
@end
@implementation GadakEmbedUIDelegate
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

static GadakEmbedUIDelegate *embedUIDelegate(void) {
	static GadakEmbedUIDelegate *d = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{ d = [[GadakEmbedUIDelegate alloc] init]; });
	return d;
}

// A bare WKWebView sends the AppleWebKit UA without the trailing
// "Version/x Safari/y" tokens real Safari appends — and Atlassian's browser
// sniffing reads exactly those, so the Confluence live editor answers
// "unsupported browser" to an engine that is literally Safari's. Filling
// applicationNameForUserAgent with the tokens makes the UA identical in shape
// to Safari's own. The version comes off the installed Safari so it tracks OS
// updates instead of rotting in a constant; the fallback only exists for a
// machine with no /Applications/Safari.app.
static NSString *safariUAFragment(void) {
	static NSString *fragment = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		NSString *version =
		    [[NSBundle bundleWithPath:@"/Applications/Safari.app"]
		        objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
		if (version == nil || version.length == 0) {
			version = @"26.0";
		}
		fragment = [[NSString stringWithFormat:@"Version/%@ Safari/605.1.15",
		                                       version] retain];
	});
	return fragment;
}

// Frames arrive in the SPA's coordinate space: CSS px with y measured from the
// top. The main webview fills the contentView 1:1 (points == CSS px), so only
// the y axis needs flipping into AppKit's bottom-left origin.
static NSRect embedRect(NSView *content, double x, double y, double w, double h) {
	double ch = content.bounds.size.height;
	return NSMakeRect(x, ch - y - h, w, h);
}

// ── Escape relay (GDK-78) ─────────────────────────────────────────────
// A keyDown inside an embedded WKWebView belongs to that page's web process:
// the SPA never sees it, so its Escape → hide-browse binding is dead exactly
// when the pane is up (⌘W works only because it is a menu accelerator). This
// monitor hands Escape back: when the first responder sits inside an embedded
// view, the event is swallowed and a synthetic Escape is posted after focus
// moves to the app's own webview, whose keydown listener runs the same path
// as if it had focus all along. The SPA stays the single owner of what
// Escape means; every other key passes through untouched.

static NSMutableSet *embedViews(void) {
	static NSMutableSet *views = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{ views = [[NSMutableSet alloc] init]; });
	return views;
}

static BOOL viewInsideEmbeds(NSView *v) {
	for (NSView *cur = v; cur != nil; cur = cur.superview) {
		if ([embedViews() containsObject:cur]) return YES;
	}
	return NO;
}

// The app's own webview, however deeply the framework wrapped it: the first
// WKWebView in the content view's tree that is not one of ours.
static WKWebView *mainWebViewIn(NSView *v) {
	for (NSView *child in v.subviews) {
		if ([child isKindOfClass:[WKWebView class]] && ![embedViews() containsObject:child]) {
			return (WKWebView *)child;
		}
	}
	for (NSView *child in v.subviews) {
		WKWebView *found = mainWebViewIn(child);
		if (found != nil) return found;
	}
	return nil;
}

static NSEvent *gadakEscapeMonitor(NSEvent *event) {
	if (event.keyCode != 53) return event;  // kVK_Escape
	NSWindow *win = event.window;
	if (win == nil) return event;
	id fr = win.firstResponder;
	if (![fr isKindOfClass:[NSView class]]) return event;
	if (!viewInsideEmbeds((NSView *)fr)) return event;
	WKWebView *main = mainWebViewIn(win.contentView);
	if (main != nil) {
		[win makeFirstResponder:main];
	}
	NSEvent *synthetic = [NSEvent
	    keyEventWithType:NSEventTypeKeyDown
	                location:NSZeroPoint
	           modifierFlags:0
	               timestamp:[NSDate timeIntervalSinceReferenceDate]
	            windowNumber:win.windowNumber
	                 context:nil
	              characters:@"\x1b"
	 charactersIgnoringModifiers:@"\x1b"
	               isARepeat:NO
	                 keyCode:53];
	if (synthetic != nil) {
		[NSApp postEvent:synthetic atStart:YES];
	}
	return nil;  // swallowed; the relayed copy is on its way to the SPA
}

static void embedInstallEscapeRelay(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
		                                      handler:^NSEvent *(NSEvent *event) {
		                                          return gadakEscapeMonitor(event);
		                                      }];
	});
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
		config.applicationNameForUserAgent = safariUAFragment();
		wv = [[WKWebView alloc] initWithFrame:embedRect(content, x, y, w, h)
		                        configuration:config];
		wv.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		wv.UIDelegate = embedUIDelegate();
		wv.hidden = YES;
		[embedViews() addObject:wv];
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
		[embedViews() removeObject:wv];
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

// paneSupported is the one fact "does this build have an in-app browse
// pane". UI that only acts on the pane (the Close Tab menu item) derives
// from it, so a platform without the pane never grows dead chrome (GDK-351).
const paneSupported = true

// installEscapeRelay is darwin's answer to GDK-78; other platforms have no
// embedded webviews to steal keystrokes in the first place.
func installEscapeRelay() {
	C.embedInstallEscapeRelay()
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
