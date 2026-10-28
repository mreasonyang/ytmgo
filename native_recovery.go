package main

import "github.com/egoist/mygo/ui"

func nativeRecoveryHelp(c *ui.Context, notice recoveryNotice, a ViewActions, p nativePalette) ui.Element {
	b := nativePill(c, a.Tr("nativeRecoveryHelp"), p, false).PaddingX(10).Shrink(0).Disabled(a.ShowRecovery == nil)
	if b.Clicked() {
		a.ShowRecovery(notice)
	}
	return b
}

func nativeRecoveryDialog(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	if !m.Recovery.valid() {
		return
	}
	title := recoveryMessageKey(m.Recovery.Kind)
	body := "nativeVerificationHelp"
	switch m.Recovery.Kind {
	case recoveryConsent:
		body = "nativeConsentHelp"
	case recoverySignIn:
		body = "nativeServiceSignInHelp"
	case recoveryRedirect:
		body = "nativeServiceRedirectHelp"
	}
	ui.DialogBase(c.Key("service-recovery"), &m.RecoveryOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.40))
		panel.Width(min(520, width-24)).Height(min(580, height-32)).Padding(20).Gap(16).
			Radius(16).Background(p.surface).Border(1, p.line).
			Shadow(0, 20, 48, 0, ui.RGBA(0, 0, 0, 0.4)).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, a.Tr(title)).Grow(1).MinWidth(0).FontSize(22).FontWeight(720).Role(ui.RoleHeading)
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 36).Clicked() {
					m.RecoveryOpen = false
				}
			})
			ui.Scroll(c.Key("recovery-details")).Grow(1).MinHeight(0).Gap(14).Children(func() {
				ui.Text(c, a.Tr(body)).FontSize(14).LineHeight(1.5)
				ui.Text(c, a.Tr("nativeRecoveryLimits")).FontSize(13).LineHeight(1.5).TextColor(p.secondary)
				hint := "nativeRecoverySignInHint"
				if m.Recovery.Scope == recoveryAccountConnect {
					hint = "nativeRecoveryConnectionHint"
				} else if m.SignedIn {
					hint = "nativeRecoveryConnectedHint"
				}
				ui.Text(c, a.Tr(hint)).FontSize(13).LineHeight(1.5).TextColor(p.secondary)
				if m.RecoveryBrowserFailed {
					ui.Text(c, a.Tr("nativeAccountBrowserError")).FontSize(13).LineHeight(1.5).TextColor(p.accent)
				}
			})
			ui.Column(c).Gap(8).Shrink(0).Children(func() {
				if nativePill(c, a.Tr("nativeAccountOpenMusic"), p, true).Disabled(a.OpenRecoveryBrowser == nil).Clicked() {
					a.OpenRecoveryBrowser()
				}
				ui.Row(c).Gap(8).Children(func() {
					retry := "buttonRetry"
					if m.Recovery.Scope == recoveryAccountConnect {
						retry = "nativeRecoveryReconnect"
					}
					if nativePill(c, a.Tr(retry), p, false).Grow(1).Disabled(a.RetryRecovery == nil).Clicked() {
						a.RetryRecovery()
					}
					if !m.SignedIn && m.Recovery.Scope != recoveryAccountConnect {
						if nativePill(c, a.Tr("shellSignIn"), p, false).Grow(1).Disabled(a.RecoveryAccount == nil).Clicked() {
							a.RecoveryAccount()
						}
					}
				})
			})
		})
	}).Label(a.Tr(title))
}
