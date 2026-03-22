# firectl

A CLI tool to import bank transactions from CSV files into [Firefly III](https://www.firefly-iii.org/).

## Usage

Build the binary:

```bash
make build
```

Configure your Firefly III instance (create a `.env` file):

```bash
FIREFLY_URL=https://your-firefly-instance.com
FIREFLY_TOKEN=your-api-token
```

Import transactions from a CSV file:

```bash
./bin/firectl --provider satispay transactions.csv
./bin/firectl --provider sanpaolo statements.csv
```

Preview without importing:

```bash
./bin/firectl --provider satispay --dry-run transactions.csv
```
