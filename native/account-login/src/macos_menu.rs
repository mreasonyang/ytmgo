//! Standard responder-chain actions keep text editing shortcuts working in WKWebView.
use objc2::{MainThreadMarker, MainThreadOnly, sel};
use objc2_app_kit::{NSApplication, NSEventModifierFlags, NSMenu, NSMenuItem};
use objc2_foundation::NSString;

pub fn install() {
    let Some(mtm) = MainThreadMarker::new() else {
        return;
    };
    let bar = NSMenu::new(mtm);
    let app = NSMenu::initWithTitle(NSMenu::alloc(mtm), &NSString::from_str("ytmgo"));
    let app_item = NSMenuItem::new(mtm);
    app_item.setSubmenu(Some(&app));
    bar.addItem(&app_item);
    // Use the window's close request so cancellation still emits the protocol reply.
    let close = unsafe {
        NSMenuItem::initWithTitle_action_keyEquivalent(
            NSMenuItem::alloc(mtm),
            &NSString::from_str("Close Login"),
            Some(sel!(performClose:)),
            &NSString::from_str("q"),
        )
    };
    app.addItem(&close);

    let edit = NSMenu::initWithTitle(NSMenu::alloc(mtm), &NSString::from_str("Edit"));
    let edit_item = NSMenuItem::new(mtm);
    edit_item.setSubmenu(Some(&edit));
    bar.addItem(&edit_item);
    for (title, action, key, shift) in [
        ("Undo", sel!(undo:), "z", false),
        ("Redo", sel!(redo:), "z", true),
        ("Cut", sel!(cut:), "x", false),
        ("Copy", sel!(copy:), "c", false),
        ("Paste", sel!(paste:), "v", false),
        ("Select All", sel!(selectAll:), "a", false),
    ] {
        // All selectors are AppKit's standard one-argument responder actions.
        let item = unsafe {
            NSMenuItem::initWithTitle_action_keyEquivalent(
                NSMenuItem::alloc(mtm),
                &NSString::from_str(title),
                Some(action),
                &NSString::from_str(key),
            )
        };
        let modifiers = NSEventModifierFlags::Command
            | if shift {
                NSEventModifierFlags::Shift
            } else {
                NSEventModifierFlags::empty()
            };
        item.setKeyEquivalentModifierMask(modifiers);
        edit.addItem(&item);
    }
    NSApplication::sharedApplication(mtm).setMainMenu(Some(&bar));
}
