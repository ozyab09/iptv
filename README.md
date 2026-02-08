# IPTV M3U Playlist Filter

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Python 3.14](https://img.shields.io/badge/python-3.14-blue.svg)](https://www.python.org/downloads/release/python-3140/)

A robust and secure IPTV M3U playlist filtering application that downloads M3U playlists from a specified URL, filters out channels based on predefined categories, processes channel names according to specific rules, and uploads the filtered playlist to S3-compatible storage.

## 🌍 Public Project Notice

This is a **public open-source project** available for anyone to use, modify, and contribute to. The code is completely transparent and accessible to all users.

### How to Use This Project

Since this is a public project with shared infrastructure, **please fork this repository** to customize it for your own needs:

1. **Fork the repository** to your own GitHub account
2. **Customize the channel categories** in `src/m3u_simple_filter/config.py` to match your preferences
3. **Configure your own environment variables** with your personal M3U source and S3 storage
4. **Deploy your own instance** with your own credentials

### Default Categories

The default categories included in this repository are examples. When you fork the project, you should modify the `CATEGORIES_TO_KEEP` list in `src/m3u_simple_filter/config.py` to include only the categories you want:

```python
# Categories to keep (modify this list in your fork)
CATEGORIES_TO_KEEP: List[str] = [
    "Your Category 1",
    "Your Category 2",
    # Add your preferred categories here
]
```

## 🚀 Features

- **Secure Download**: Safely downloads M3U playlists with size validation and injection protection
- **Category Filtering**: Keeps only channels from specified categories
- **Channel Name Processing**:
  - Removes 'orig' suffix from channel names
  - Keeps only HD versions when both HD and non-HD versions exist
- **Smart Filtering**: Excludes channels matching specific regex patterns (e.g., `+1` channels)
- **Cloud Storage**: Uploads filtered playlists to S3-compatible storage
- **Dry-Run Mode**: Test functionality without uploading to S3
- **Comprehensive Logging**: Detailed logs with before/after statistics and file sizes in KB
- **Optimized File Sizes**: Efficient compression to keep EPG files within typical S3 size limits (3-5 MB range)
- **Organized File Management**: All downloaded and processed files saved in a dedicated output directory
- **Full Test Coverage**: Unit tests for all core functionality
- **CI/CD Pipeline**: Automated testing and deployment via GitHub Actions
- **Public & Customizable**: Easy to fork and customize for your own needs

## 🛠️ Architecture

The application follows a modular design with clear separation of concerns:

```
iptv/
├── src/
│   ├── m3u_simple_filter/          # Main application modules
│   │   ├── __init__.py             # Package initialization
│   │   ├── config.py               # Configuration management with validation
│   │   ├── m3u_processor.py        # M3U download, parsing and filtering
│   │   ├── s3_operations.py        # S3 upload operations
│   │   └── main.py                 # Application entry point
│   └── run_filter.py               # Script entry point
├── tests/                          # Unit tests
│   ├── test_config.py              # Configuration tests
│   ├── test_m3u_processor.py       # M3U processing tests
│   ├── test_s3_operations.py       # S3 operations tests
│   └── test_main.py                # Main application tests
├── output/                         # Directory for saving processed files (created at runtime)
├── .github/
│   └── workflows/
│       └── filter-m3u.yml          # GitHub Actions workflow
├── pyproject.toml                  # Project configuration and dependencies
├── test.sh                         # Local testing script
└── README.md                       # Project documentation
```

## 📋 Prerequisites

- Python 3.14+
- S3-compatible storage account (any provider) with appropriate size limits configured
- AWS CLI credentials (for S3 access)

## 🛠️ Installation

1. Clone the repository:
```bash
git clone https://github.com/ozyab09/iptv.git
cd iptv
```

2. Create and activate a virtual environment:
```bash
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
```

3. Install dependencies:
```bash
pip install -e .
# Or for development:
pip install -e ".[dev]"
```

## ⚙️ Configuration

The application uses environment variables for configuration. Create a `.env` file in the project root or set these variables in your environment:

| Environment Variable | Description | Default Value |
|----------------------|-------------|---------------|
| `M3U_SOURCE_URL` | Source URL for the M3U playlist | `https://your-provider.com/playlist.m3u` |
| `S3_BUCKET_NAME` | S3 bucket name | `your-bucket-name` |
| `S3_OBJECT_KEY` | S3 object key | `playlist.m3u` |
| `S3_ENDPOINT_URL` | S3-compatible storage endpoint URL | `https://s3.amazonaws.com` |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_EPG_KEY` | S3 object key for EPG file | `epg.xml.gz` |
| `EPG_SOURCE_URL` | Source URL for the EPG XML file | `https://your-epg-provider.com/epg.xml.gz` |
| `LOCAL_EPG_PATH` | Local path for downloaded EPG file | `epg.xml.gz` |
| `OUTPUT_DIR` | Directory for saving processed files | `output` |
| `DRY_RUN` | Run in dry-run mode | (unset) |
| `AWS_ACCESS_KEY_ID` | S3-compatible storage access key | (required) |
| `AWS_SECRET_ACCESS_KEY` | S3-compatible storage secret key | (required) |

### Environment Setup

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit the `.env` file with your specific configuration:
   ```bash
   nano .env
   ```

3. Set the required values:
   - `M3U_SOURCE_URL`: Your M3U playlist source URL
   - `S3_BUCKET_NAME`: Your S3 bucket name
   - `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`: Your S3-compatible storage credentials
   - `S3_ENDPOINT_URL`: Your S3-compatible storage endpoint (e.g., for Yandex Cloud: `https://storage.yandexcloud.net`)
   - `S3_REGION`: Your S3 region
   - `S3_EPG_KEY`: S3 object key for EPG file (default: `epg.xml.gz`)
   - `S3_OBJECT_KEY`: S3 object key for playlist (default: `playlist.m3u`)
   - `OUTPUT_DIR`: Directory for saving processed files (default: `output`)
   - Optionally update other values as needed

4. **Important**: Ensure your S3-compatible storage has appropriate size limits configured to accommodate the processed EPG file (typically 3-5 MB after filtering and compression)

5. Load the environment variables:
   ```bash
   export $(grep -v '^#' .env | xargs)
   ```

   Or use a tool like `direnv` to automatically load environment variables.

## 🧪 Testing

Run the unit tests:
```bash
python -m pytest tests/ -v
```

Run with coverage:
```bash
python -m coverage run -m pytest tests/
python -m coverage report
```

Run local test scenarios:
```bash
./test.sh
```

## 🚀 Usage

### Fork and Customize

Since this is a public project, to use it for your own needs:

1. **Fork this repository** to your GitHub account
2. **Customize the channel categories** in `src/m3u_simple_filter/config.py`
3. **Set up your environment variables** with your personal M3U source and S3 storage credentials

### Local Execution

To run the script locally:

```bash
cd src
python run_filter.py
```

For dry-run mode (saves filtered playlist locally without uploading to S3):

```bash
DRY_RUN=true python run_filter.py
```

To run with S3 upload (ensure your S3 storage has appropriate size limits configured):

```bash
# Make sure DRY_RUN is not set or commented out in your .env file
python run_filter.py
```

### GitHub Actions

When you fork this repository, the workflow will run in these scenarios:
- **Scheduled runs**: Normal execution with S3 upload (configure as needed in your fork)
- **Pull requests**: Dry-run execution for testing
- **Main branch pushes**: Normal execution with S3 upload

⚠️ **Note**: The default workflow in this public repository is configured with placeholder values. When you fork the repository, you'll need to set up your own GitHub Secrets with your personal credentials.

## 🔐 Security Features

- **Input Validation**: Validates M3U source URL and S3 configuration
- **Size Limiting**: Maximum file size validation (100MB by default)
- **Injection Prevention**: Sanitizes extremely long lines that might be malicious
- **Environment Validation**: Validates configuration before processing
- **Secure Credential Handling**: Uses environment variables for sensitive data
- **Log Sanitization**: Automatically masks sensitive information in logs to prevent accidental exposure
- **Privacy Protection**: All personal/sensitive data is externalized to environment variables

## 📊 Performance Optimizations

- **Chunked Downloads**: Downloads large files in chunks to prevent memory issues
- **Efficient Filtering**: Optimized algorithms for category and channel processing
- **Caching**: GitHub Actions workflow includes dependency caching

## 🤝 Contributing

For personal use, we recommend **forking** this repository to customize it for your own needs.

For contributing improvements back to the main project:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Write unit tests for new functionality
- Follow PEP 8 coding standards
- Include docstrings for all functions and classes
- Update documentation as needed

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🐛 Issues and Support

If you encounter any issues or have questions, please [open an issue](https://github.com/ozyab09/iptv/issues) on GitHub.

## 🙏 Acknowledgments

- Thanks to the open-source community for the tools and libraries that made this project possible
- Special thanks to contributors who help maintain and improve this project