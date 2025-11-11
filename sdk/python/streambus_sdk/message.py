"""Message types for StreamBus SDK."""

from dataclasses import dataclass
from datetime import datetime
from typing import Optional, Dict


@dataclass
class Message:
    """A message received from or sent to StreamBus."""

    topic: str
    partition: int
    offset: int
    key: Optional[bytes]
    value: bytes
    timestamp: datetime
    headers: Optional[Dict[str, str]] = None

    def value_as_str(self, encoding: str = 'utf-8', errors: str = 'replace') -> str:
        """Get value as string."""
        return self.value.decode(encoding, errors=errors)

    def key_as_str(self, encoding: str = 'utf-8', errors: str = 'replace') -> Optional[str]:
        """Get key as string."""
        if self.key:
            return self.key.decode(encoding, errors=errors)
        return None

    def __repr__(self) -> str:
        return (
            f"Message(topic={self.topic}, partition={self.partition}, "
            f"offset={self.offset}, key={self.key_as_str()})"
        )
