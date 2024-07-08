import requests
import yaml

# Constants
URL = "https://alpha.c930.net/api/users/login"
YAML_FILE = "integration-tests/variables.yaml"

def get_userdata():
    with open(YAML_FILE, "r") as file:
        data = yaml.safe_load(file)
    
    return data.get("username1"), data.get("password1"), data.get("username2"), data.get("password2")

def get_jwt_token(username, password):
    response = requests.post(URL, json={"username": username, "password": password})
    if response.status_code == 200:
        token = response.json().get("token")
        return token
    else:
        raise Exception("Failed to obtain JWT token")

def update_yaml_file_with_token(number, token):
    with open(YAML_FILE, "r") as file:
        data = yaml.safe_load(file)
    
    data["jwt"+str(number)] = token
    
    with open(YAML_FILE, "w") as file:
        yaml.dump(data, file, default_flow_style=False)

if __name__ == "__main__":
    try:
        username1, password1, username2, password2 = get_userdata()
        token = get_jwt_token(username1, password1)
        update_yaml_file_with_token(1, token)
        token = get_jwt_token(username2, password2)
        update_yaml_file_with_token(2, token)
        print("JWT tokens updated successfully.")
    except Exception as e:
        print(f"Error: {e}")
