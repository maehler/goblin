bin_dir = ./bin
binary_name = goblin
main_path = ./cmd/goblin

.PHONY: all
all: goblin goblin-raspi

.PHONY: tailwind
tailwind:
	tailwindcss -i ./http/static/css/_input.css -o ./http/static/css/style.css

.PHONY: generate
generate: tailwind
	go generate ./...

.PHONY: goblin
goblin: generate
	go build -o ${bin_dir}/${binary_name} ${main_path}

.PHONY: goblin-raspi
goblin-raspi: generate
	GOOS=linux GOARCH=arm GOARM=6 go build -o ${bin_dir}/${binary_name}-raspi ${main_path}

.PHONY: clean
clean:
	rm -f goblin-raspi
