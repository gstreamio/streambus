"""
StreamBus Python SDK

A high-level Python SDK for StreamBus that provides simplified interfaces
for producing and consuming messages.
"""

from ._wire import (
    ISOLATION_READ_COMMITTED,
    ISOLATION_READ_UNCOMMITTED,
    BrokerError,
    ProtocolError,
)
from .client import StreamBusClient, connect
from .producer import Producer
from .consumer import Consumer
from .message import Message

__version__ = "0.2.0"

__all__ = [
    "StreamBusClient",
    "connect",
    "Producer",
    "Consumer",
    "Message",
    "BrokerError",
    "ProtocolError",
    "ISOLATION_READ_UNCOMMITTED",
    "ISOLATION_READ_COMMITTED",
]
