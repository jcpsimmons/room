.PHONY: dev build start

MARKETING_SITE_DIR := marketing-site

dev:
	cd $(MARKETING_SITE_DIR) && bun run dev

build:
	cd $(MARKETING_SITE_DIR) && bun run build

start:
	cd $(MARKETING_SITE_DIR) && bun run start
