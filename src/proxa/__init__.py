"""
Proxa - Proxy ASGI para escalado dinámico de workers
"""

__version__ = "0.1.0"

from proxa.core import run_proxa
from proxa.servers import get_server_adapter
from proxa.process import WorkerInfo

__all__ = ['run_proxa', 'get_server_adapter', 'WorkerInfo']