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
