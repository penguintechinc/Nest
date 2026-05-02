"""Entry point for the manager service."""

import os

from app import create_app


if __name__ == "__main__":
    app = create_app()
    port = int(os.getenv("PORT", "8081"))
    app.run(host="0.0.0.0", port=port, debug=False)
