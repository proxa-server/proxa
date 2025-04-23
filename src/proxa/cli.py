import click
import logging
import anyio
import psutil
from typing import Optional

# Core module imports
from .config import load_config, merge_config
from .core import run_proxa

# Configuración de niveles de logging
LOG_LEVELS = {
    'error': logging.ERROR,
    'warning': logging.WARNING,
    'info': logging.INFO,
    'debug': logging.DEBUG
}

def setup_logging(verbosity: int, log_level: str):
    """Configure logging based on verbosity and level"""
    base_level = LOG_LEVELS.get(log_level.lower(), logging.INFO)
    
    # Ajustar el nivel basado en la verbosidad
    if verbosity > 0:
        if verbosity == 1:  # -v
            base_level = min(base_level, logging.INFO)
        elif verbosity >= 2:  # -vv
            base_level = logging.DEBUG
    
    logging.basicConfig(
        level=base_level,
        format='%(asctime)s - %(levelname)s - %(message)s' if verbosity > 0 else '%(levelname)s: %(message)s'
    )
    return logging.getLogger(__name__)

def calculate_default_workers():
    """Calculate default number of workers based on CPU cores"""
    cpu_count = psutil.cpu_count(logical=False)
    return max(2, cpu_count)

@click.group()
def cli():
    """Proxa - Intelligent ASGI/WSGI Server Autoscaler"""
    pass

@cli.command()
@click.option('-v', '--verbose', count=True,
              help='Increase output verbosity (-v or -vv).')
@click.option('--log-level', type=click.Choice(['error', 'warning', 'info', 'debug'], case_sensitive=False),
              default='info', help='Set the logging level.')
@click.option('--server', type=click.Choice(['uvicorn', 'hypercorn', 'granian']),
              help='ASGI/WSGI server to use.')
@click.option('--app', type=str,
              help='Application to run (e.g., myapp:app).')
@click.option('--workers', type=int,
              help='Initial number of workers (default: auto-detected based on CPU cores).')
@click.option('--min-workers', type=int,
              help='Minimum number of workers (default: 1).')
@click.option('--max-workers', type=int,
              help='Maximum number of workers (default: 2x CPU cores).')
@click.option('--max-cpu', type=float, default=80.0,
              help='Maximum average CPU percentage before scaling (default: 80.0).')
@click.option('--max-mem', type=float,
              help='Maximum memory (MB) per worker before scaling (optional).')
@click.option('--enable-health', is_flag=True,
              help='Enable health/metrics endpoint.')
@click.option('--health-host', type=str, default='127.0.0.1',
              help='Host for health endpoint.')
@click.option('--health-port', type=int, default=9000,
              help='Port for health endpoint.')
@click.option('-c', '--config', type=click.Path(exists=True),
              help='Path to configuration file (.toml or .conf).')
def run(verbose, log_level, server, app, workers, min_workers, max_workers, max_cpu, max_mem,
        enable_health, health_host, health_port, config):
    """Run and autoscale the specified application.

    Examples:

    Basic Usage:
        proxa run --server uvicorn --app myapp:app

    With Verbosity:
        proxa run -v --server uvicorn --app myapp:app
        proxa run -vv --server uvicorn --app myapp:app --log-level debug

    With Custom Log Level:
        proxa run --log-level warning --server uvicorn --app myapp:app
    """
    try:
        # Configurar logging primero
        logger = setup_logging(verbose, log_level)
        
        if config:
            file_config = load_config(config)
            logger.debug(f"Loaded configuration from file: {config}")
        else:
            file_config = {}

        # Calculate default values based on system resources
        default_workers = calculate_default_workers()
        logger.debug(f"Calculated default workers: {default_workers}")
        
        # Set smart defaults if not provided
        if not workers:
            workers = default_workers
        if not min_workers:
            min_workers = 1
        if not max_workers:
            max_workers = default_workers * 2

        cli_config = {
            'server': server,
            'app': app,
            'workers': workers,
            'min_workers': min_workers,
            'max_workers': max_workers,
            'max_cpu': max_cpu,
            'max_mem': max_mem,
            'enable_health': enable_health,
            'health_host': health_host,
            'health_port': health_port
        }

        logger.info("Starting Proxa with configuration...")
        if verbose > 0:
            logger.debug(f"Configuration details: {cli_config}")
            
        config = merge_config(file_config, cli_config)
        anyio.run(run_proxa, config)
    except Exception as e:
        logger.error(f"Error running Proxa: {str(e)}")
        click.echo(f"Error: {str(e)}", err=True)
        raise click.Abort()

def main():
    cli()

if __name__ == '__main__':
    main()