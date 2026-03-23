documan:
	docker compose build --progress=plain documan && docker compose up -d documan

documan-import:
	docker compose exec -t documan /documan/bin/documan import

documan-lint:
	docker compose exec -t documan /documan/bin/documan lint

documan-fix:
	docker compose exec -t documan /documan/bin/documan fix

documan-vectorize:
	docker compose exec -t documan /documan/bin/documan vectorize
