# Configuration variables
IMAGE_NAME = telegram-bot
CONTAINER_NAME = telegram-bot-container
DATA_DIR = $(HOME)/telegram-bot-data
DOCKER_VOLUME_PATH = /main/data


# Create data directory
setup:
	mkdir -p $(DATA_DIR)

# Build the Docker image
build:
	docker build -t $(IMAGE_NAME) .

# Run the container with auto-restart and volume mounting
run: setup
	docker run -d \
		--name $(CONTAINER_NAME) \
		--restart always \
		-v $(DATA_DIR):/$(DOCKER_VOLUME_PATH) \
		-e TELEGRAM_TOKEN=$(TELEGRAM_TOKEN) \
		-e BOT_PASSWORD=$(BOT_PASSWORD) \
		$(IMAGE_NAME)

# Stop the container
stop:
	docker stop $(CONTAINER_NAME)

# Start an existing container
start:
	docker start $(CONTAINER_NAME)

# Restart the container
restart:
	docker restart $(CONTAINER_NAME)

# Remove the container
remove:
	docker rm -f $(CONTAINER_NAME) || true

# View container logs
logs:
	docker logs $(CONTAINER_NAME)

# Follow container logs
logs-follow:
	docker logs -f $(CONTAINER_NAME)

# Show container status
status:
	docker ps -a | grep $(CONTAINER_NAME)


# Clean everything (remove container and image)
clean: remove
	docker rmi $(IMAGE_NAME) || true
	@echo "To remove data directory run: rm -rf $(DATA_DIR)"

# Build and run (all-in-one command)
deploy: remove build run

.PHONY: setup build run stop start restart remove logs logs-follow status rebuild clean deploy
