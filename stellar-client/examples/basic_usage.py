#!/usr/bin/env python3
"""
Basic Stellar Client Usage Example

This example demonstrates the fundamental operations with Stellar client:
- Connecting to a Stellar node
- Listing discovered devices
- Pinging devices
- Managing security policies
"""

from stellar_client import StellarClient
from stellar_client.exceptions import StellarException

def get_first_device(client: StellarClient):
    return next(iter(client.list_devices()), None)

def main():
    print("Stellar Client - Basic Usage Example")
    print("=" * 40)
    
    try:
        # Initialize client and connect to Stellar node
        with StellarClient() as client:
            print("✓ Connected to Stellar node")
            
            # Get node information
            node_info = client.get_node_info()
            print(f"Node ID: {node_info.id}")
            print(f"Addresses: {node_info.addresses}")
            print()
            
            # List discovered devices
            print("Discovering devices...")
            devices = client.list_devices()
            print(f"Found {len(devices)} devices:")
            
            for i, device in enumerate(devices, 1):
                print(f"  {i}. {device.id}")
                if device.sys_info:
                    print(f"     Platform: {device.sys_info.platform}, CPU: {device.sys_info.cpu}")
            print()
            
            if not devices:
                print("No devices found. Start additional Stellar nodes to see them here.")
                return
                
            # Ping devices
            print("Pinging devices...")
            for device in devices:
                try:
                    ping_result = client.echo.ping(device.id)
                    print(f"  ✓ {device.id}: {ping_result.status}")
                except Exception as e:
                    print(f"  ✗ {device.id}: Failed - {e}")
            print()
            
            # Get detailed device information
            if devices:
                print("Getting detailed device info...")
                try:
                    device_info = client.echo.get_device_info(get_first_device(client).id)
                    assert device_info, "Device not found"
                    
                    print(f"Device:")
                    print(f"Reference Token: {device_info.reference_token}")
                    if device_info.sys_info:
                        sys_info = device_info.sys_info
                        print(f"System Info:")
                        print(f"  - Platform: {sys_info.platform}")
                        print(f"  - CPU: {sys_info.cpu}")
                        print(f"  - GPU: {sys_info.gpu}")
                        if sys_info.ram:
                            print(f"  - Memory: {sys_info.ram // 1024} GB")
                except Exception as e:
                    print(f"  Failed to get device info: {e}")
                print()
            
            # Security policy management
            print("Security Policy Management:")
            try:
                policy = client.get_policy()
                print(f"Policy enabled: {policy.enable}")
                print(f"Whitelisted devices: {len(policy.whitelist)}")
                
                if devices and policy.enable:
                    # Add a device to whitelist
                    device_id = get_first_device(client).id
                    if device_id not in policy.whitelist:
                        print(f"Adding {device_id} to whitelist...")
                        success = client.whitelist_device(device_id)
                        print(f"  {'✓' if success else '✗'} Whitelist update: {success}")
                        
                        # Verify whitelist
                        updated_whitelist = client.get_whitelist()
                        print(f"Current whitelist: {len(updated_whitelist)} devices")
                        
            except Exception as e:
                print(f"Policy management error: {e}")
            print()
            
        print("✓ Disconnected from Stellar node")
        
    except StellarException as e:
        print(f"Stellar Error: {e}")
        return 1
    except Exception as e:
        print(f"Unexpected error: {e}")
        return 1
        
    return 0


if __name__ == "__main__":
    exit(main())