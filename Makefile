# WORLD_DIR points at the authored zone tree the loader populates the DB
# from on first boot. Override at the make-call site (e.g.
# `make run/server WORLD_DIR=/path/to/other`) to load a different tree.
# The variable is exported so build/server (which only compiles) is
# unaffected, but run/server and run/live/server pick it up.
#
# Top-level config can also be set via a YAML file passed with
# `-config <path>` (see config.example.yaml). Env vars override the file.
WORLD_DIR ?= ./data/world
export WORLD_DIR

.PHONY: build/server
build/server:
	go build -o=/tmp/bin/server cmd/server/main.go

.PHONY: run/server
run/server: build/server
	/tmp/bin/server

.PHONY: run/live/server
run/live/server:
	go mod tidy;go run github.com/cosmtrek/air@v1.43.0 \
		--build.cmd "make build/server" --build.bin "/tmp/bin/server" --build.delay "100" \
		--build.exclude_dir "" \
		--build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
		--misc.clean_on_exit "true"

.PHONY: zip
zip:
	zip -r /tmp/project.zip . \
		-x "*.git*" "*.git/**" "project.zip" "*.env" "*.env.*" \
		"node_modules/*" "tmp/*" "/tmp/*"

# WHEELMUD_HOST / WHEELMUD_PORT stamp the generated Mudlet profile so
# a player who imports it lands on the right server with one click.
# Defaults match the dev workflow (localhost:2323); override on the
# command line for a public release:
#   make mudlet-package WHEELMUD_HOST=mymud.example.com WHEELMUD_PORT=2323
WHEELMUD_HOST ?= localhost
WHEELMUD_PORT ?= 2323

.PHONY: mudlet-package
mudlet-package:
	@mkdir -p dist/mudlet
	@sed -e 's/__WHEELMUD_HOST__/$(WHEELMUD_HOST)/g' \
		-e 's/__WHEELMUD_PORT__/$(WHEELMUD_PORT)/g' \
		clients/mudlet/src/profile.xml.template > dist/mudlet/wheelmud.profile
	@cd clients/mudlet/src && \
		rm -f ../../../dist/mudlet/wheelmud.mpackage && \
		zip -q ../../../dist/mudlet/wheelmud.mpackage \
			config.lua init.lua mapper.lua vitals.lua status.lua chat.lua
	@echo "wrote dist/mudlet/wheelmud.mpackage"
	@echo "wrote dist/mudlet/wheelmud.profile (host=$(WHEELMUD_HOST), port=$(WHEELMUD_PORT))"
