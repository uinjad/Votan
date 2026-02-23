package config

import "time"

const (
	// Налаштування карти
	MaxX = 20
	MaxY = 35

	// Налаштування гри
	TickRate        = 500 * time.Millisecond
	MaxStepsPerTurn = 33
	PlayerTimeout   = -3 * time.Hour
	ChatBubbleTTL   = 7 * time.Second

	// Секрети та Адмінка
	AdminSecret = "GOD_MODE_ADMIN_SECRET"

	// Івенти
	VoteDuration     = 60 * time.Second
	VoteResultTTL    = 10 * time.Second
	Attack5GDuration = 20 * time.Second
	Debuff5GDuration = 30 * time.Second

	// Битва з Ящером (Boss)
	BossMaxHP     = 50 // 5 ударів по 10 дамагу
	BossHitDamage = 10
	BossResultTTL = 7 * time.Second

	// Ліміти кастомізації
	MaxHeadID = 16
	MaxBodyID = 14
)
