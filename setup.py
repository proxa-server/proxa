from setuptools import setup, find_packages

setup(
    name="proxa",
    version="0.1.0",
    packages=find_packages(where="src"),
    package_dir={"": "src"},
    install_requires=[
        "anyio>=3.0.0",
        "httpx>=0.24.0",
        "psutil>=5.9.0",
        "msgspec>=0.19.0"
        # ... otras dependencias existentes ...
    ],
    # ... resto de la configuración ...
)