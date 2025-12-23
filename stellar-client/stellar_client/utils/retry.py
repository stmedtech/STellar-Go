"""Retry mechanism utilities."""

import asyncio
import time
from typing import Callable, Any, Type, Union, Tuple
from ..exceptions import ConnectionError, TimeoutError


class RetryPolicy:
    """Retry policy for handling transient failures."""
    
    def __init__(
        self,
        max_retries: int = 3,
        backoff_factor: float = 1.5,
        max_backoff: float = 60.0,
        retry_on: Tuple[Type[Exception], ...] = (ConnectionError, TimeoutError),
    ):
        """Initialize retry policy.
        
        Args:
            max_retries: Maximum number of retry attempts
            backoff_factor: Exponential backoff multiplier
            max_backoff: Maximum backoff time in seconds
            retry_on: Exception types that should trigger retries
        """
        self.max_retries = max_retries
        self.backoff_factor = backoff_factor
        self.max_backoff = max_backoff
        self.retry_on = retry_on
        
    def execute_with_retry(self, func: Callable, *args, **kwargs) -> Any:
        """Execute function with retry logic (synchronous)."""
        last_exception = None
        
        for attempt in range(self.max_retries + 1):
            try:
                return func(*args, **kwargs)
            except self.retry_on as e:
                last_exception = e
                if attempt == self.max_retries:
                    break
                    
                backoff_time = min(
                    self.backoff_factor ** attempt,
                    self.max_backoff
                )
                time.sleep(backoff_time)
                
        raise last_exception
        
    async def execute_with_retry_async(self, func: Callable, *args, **kwargs) -> Any:
        """Execute function with retry logic (asynchronous)."""
        last_exception = None
        
        for attempt in range(self.max_retries + 1):
            try:
                if asyncio.iscoroutinefunction(func):
                    return await func(*args, **kwargs)
                else:
                    return func(*args, **kwargs)
            except self.retry_on as e:
                last_exception = e
                if attempt == self.max_retries:
                    break
                    
                backoff_time = min(
                    self.backoff_factor ** attempt,
                    self.max_backoff
                )
                await asyncio.sleep(backoff_time)
                
        raise last_exception