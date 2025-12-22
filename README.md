# fetch-prs

## Description

`fetch-prs` is a tool to fetch and list your GitHub pull requests across multiple repositories within a custom date range.

## Features

- Fetch pull requests from a GitHub repository.
- Output data in various formats (e.g., JSON, plain text).
- Lightweight and easy to integrate into existing workflows.

## Prerequisites

- Go 1.25+ installed on your system.
- A GitHub personal access token with the necessary permissions.

## Installation

1. Clone the repository:
   ```bash
   git clone git@github.com:mehedygroup/fetch-prs.git
   cd fetch-prs
   ```

2. Build the project:
   ```bash
   go build -o fetch-prs
   ```

3. Run the binary:
   ```bash
   ./fetch-prs
   ```

## Usage

1. Create a `.env` file in the root directory and add your GitHub token:
   ```env
   GITHUB_USERNAME=username
   GITHUB_TOKEN=your_personal_access_token
   REPOS=repo-org/repo1,repo-org2/repo2
   ```

2. Run the tool with the desired date range:
   ```bash
   ./fetch-prs fetch 2025-12-01 2025-12-15 --output json
   ```

   Example options:
   - `2025-12-01`: Start date for fetching pull requests.
   - `2025-12-15`: End date for fetching pull requests.

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Commit your changes and open a pull request.

## License

This project is licensed under the Apache 2.0 License. See the `LICENSE` file for details.
