package app

// The sizing half of the Islands design tokens; theme.go is the colour half.
// Components read these through oiTokens so a spacing, a radius or a height is
// chosen once, and the light and dark appearances share every value, as the
// tokens.css custom properties do.

import "github.com/egoist/mygo/ui"

const (
	oiSpaceXS float32 = 2
	oiSpaceS  float32 = 4
	oiSpaceM  float32 = 6
	oiSpaceL  float32 = 8
	oiSpaceXL float32 = 12

	oiRadiusS float32 = 4
	oiRadiusM float32 = 6
	oiRadiusL float32 = 8

	oiButtonHeight float32 = 28
	oiInputHeight  float32 = 28

	oiFontSizeS float32 = 11
	oiFontSizeM float32 = 13
	oiFontSizeL float32 = 15

	oiShadowBlur  float32 = 24
	oiShadowY     float32 = 8
	oiShadowAlpha float32 = 0.28
)

// themeTokens is the whole sizing set a component reads. The fields that name
// workbench chrome point at the workbench constants, so the tokens and the
// layout cannot drift apart.
type themeTokens struct {
	SpaceXS, SpaceS, SpaceM, SpaceL, SpaceXL float32
	RadiusS, RadiusM, RadiusL                float32

	TitleBarHeight, StatusBarHeight, HeaderHeight float32
	TabBarHeight, TabHeight                       float32
	ButtonHeight, InputHeight                     float32

	FontSizeS, FontSizeM, FontSizeL float32

	IslandGap, IslandRadius              float32
	StripeWidth, StripeButton, StripeGap float32

	ShadowBlur, ShadowY, ShadowAlpha float32
}

var oiTokens = themeTokens{
	SpaceXS: oiSpaceXS, SpaceS: oiSpaceS, SpaceM: oiSpaceM, SpaceL: oiSpaceL, SpaceXL: oiSpaceXL,
	RadiusS: oiRadiusS, RadiusM: oiRadiusM, RadiusL: oiRadiusL,

	TitleBarHeight: oiTitleBarHeight, StatusBarHeight: oiStatusBarHeight,
	HeaderHeight: oiHeaderHeight, TabBarHeight: oiTabBarHeight, TabHeight: oiTabHeight,
	ButtonHeight: oiButtonHeight, InputHeight: oiInputHeight,

	FontSizeS: oiFontSizeS, FontSizeM: oiFontSizeM, FontSizeL: oiFontSizeL,

	IslandGap: oiIslandGap, IslandRadius: oiIslandRadius,
	StripeWidth: oiStripeWidth, StripeButton: oiStripeButton, StripeGap: oiStripeGap,

	ShadowBlur: oiShadowBlur, ShadowY: oiShadowY, ShadowAlpha: oiShadowAlpha,
}

// oiTokensOf is the token set in use. Both appearances share it, as OpenIDEA's
// light theme overrides colours only.
func oiTokensOf(p *ui.Theme) themeTokens { return oiTokens }
