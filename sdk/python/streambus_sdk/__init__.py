"""
StreamBus Python SDK

A high-level Python SDK for StreamBus that provides simplified interfaces
for producing and consuming messages.
"""

from .client import StreamBusClient, connect
from .producer import Producer
from .consumer import Consumer
from .message import Message

__version__ = "0.1.0"

__all__ = [
    "StreamBusClient",
    "connect",
    "Producer",
    "Consumer",
    "Message",
]
