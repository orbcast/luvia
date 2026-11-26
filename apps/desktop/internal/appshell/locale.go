package appshell

import "luvia/apps/desktop/locales"

// localeTags are the languages the viewer may choose, in the order the
// menus and the settings drop-down list them: the source language first,
// then Simplified Chinese. The follow system choice is not among them; it
// is offered by the menu alone, and the drop-down showing the language in
// effect stands for it.
var localeTags = [...]string{"en", "zh-Hans"}

// LocaleTags returns the tags the viewer may choose, in display order.
func LocaleTags() []string { return localeTags[:] }

// LocaleName names the language of tag in the language of m, so that a
// reader of English finds 简体中文 and a reader of Chinese finds English;
// "" is the follow system choice.
func LocaleName(m locales.Messages, tag string) string {
	switch tag {
	case "":
		return m.FollowSystem()
	case "en":
		return locales.In("en").English()
	case "zh-Hans":
		return locales.In("zh-Hans").SimplifiedChinese()
	}
	return tag
}

// DefaultLocale is the language the app opens in when nothing was chosen
// and the system language matches none of those the app is written in:
// Simplified Chinese, the language of the readers the app is made for.
// MyGo itself falls back on the source language, English, which is why
// the window says so.
const DefaultLocale = "zh-Hans"
