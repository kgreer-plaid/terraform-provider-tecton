# Access policy can be imported by specifying it's ID, which is in the format
# {user|service}-{id}. For example, an access policy for a user with ID 'abc'
# will have the ID 'user-abc'.
terraform import tecton_access_policy.example user-abc